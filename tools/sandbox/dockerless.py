import argparse
import gzip
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.distribution import package  # noqa: E402
from tools.sandbox.commands import Commands  # noqa: E402
from tools.sandbox.mcp import Client  # noqa: E402

HELPERS = (
    "awk",
    "cat",
    "chmod",
    "cmp",
    "cp",
    "dirname",
    "flock",
    "git",
    "grep",
    "gzip",
    "head",
    "mkdir",
    "mktemp",
    "mv",
    "pwd",
    "rm",
    "sed",
    "sh",
    "sha256sum",
    "tar",
    "timeout",
    "uname",
    "wc",
)
FORBIDDEN = ("docker", "podman", "php", "composer", "npm", "node", "go", "curl", "wget")
TOOLS = {
    "find_reports",
    "get_report_metadata",
    "get_flat_profile",
    "get_callers",
    "get_aggregated_call_graph",
}


def restricted_path(directory):
    directory.mkdir()
    for name in HELPERS:
        target = shutil.which(name)
        if target is None:
            raise RuntimeError("Docker-less installation needs host helper: " + name)
        (directory / name).symlink_to(Path(target).resolve())
    assert all(shutil.which(name, path=str(directory)) is None for name in FORBIDDEN)


def fingerprint(directory):
    return {
        path.relative_to(directory).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in directory.rglob("*")
        if path.is_file() and ".git" not in path.parts
    }


class DockerlessSandbox(Commands):
    def __init__(self, directory, archive):
        self.root, self.archive = directory, archive
        self.project, self.logs = directory / "project", directory / "logs"
        self.project.mkdir()
        self.logs.mkdir()
        self.sequence = 0
        self.summary = {
            "environment": "dockerless",
            "installation_selection": "standalone",
            "passed": False,
        }
        self.binary = directory / "bin/phpwarden"
        self.reports = directory / "reports"
        self.producer = json.loads((ROOT / "tools/spx/sources.json").read_text())["spx"]["commit"]

    def prepare(self):
        helpers = self.root / "helpers"
        restricted_path(helpers)
        for name in ("tmp", "cache", "claude"):
            (self.root / name).mkdir()
        self.environment = os.environ | {
            "PATH": str(helpers),
            "TMPDIR": str(self.root / "tmp"),
            "XDG_CACHE_HOME": str(self.root / "cache"),
            "CLAUDE_CONFIG_DIR": str(self.root / "claude"),
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": "/dev/null",
            "GIT_TERMINAL_PROMPT": "0",
        }
        for name in ("DOCKER_HOST", "DOCKER_CONTEXT", "ENV", "BASH_ENV"):
            self.environment.pop(name, None)
        self.summary["unavailable_executables"] = list(FORBIDDEN)
        fixtures = ROOT / "tools/spx/fixtures"
        self.reports.mkdir()
        for key in ("repeated", "cached"):
            data = json.loads((fixtures / (key + ".json")).read_text())
            data.update(
                cli_command_line="/fixture/scenario.php --token=dockerless-cli-secret",
                host_name="dockerless-private-host",
                custom_metadata_str="dockerless-private-metadata",
            )
            (self.reports / (key + ".json")).write_text(json.dumps(data))
            shutil.copyfile(fixtures / (key + ".txt.gz"), self.reports / (key + ".txt.gz"))
        shutil.copyfile(fixtures / "scenario.php", self.project / "scenario.php")
        module = self.project / "web/modules/custom/demo/demo.module"
        module.parent.mkdir(parents=True)
        module.write_text("<?php\nfunction dockerless_fixture(): int { return 1; }\n")
        for name in ("composer.json", "composer.lock"):
            shutil.copyfile(ROOT / "tools/sandbox/drupal10" / name, self.project / name)
        (self.project / ".gitignore").write_text("/vendor/\n/node_modules/\n")
        self.run(["git", "init", "-q"])

    def install(self):
        self.summary["stage"] = "offline install and repeat"
        print("Docker-less: installing CLI without Docker, PHP or package managers", flush=True)
        args = ["sh", ROOT / "install.sh", "--archive", self.archive, "--dir", self.binary.parent]
        self.run(args)
        before = self.binary.read_bytes()
        self.run(args)
        self.run([*args, "--force"])
        assert before == self.binary.read_bytes()
        bad = self.root / "bad-checksums"
        bad.write_text("0" * 64 + "  phpwarden-linux-amd64.tar.gz\n")
        self.run([*args, "--checksums", bad, "--force"], expected=(3,))
        assert before == self.binary.read_bytes()
        metadata = json.loads(self.cli("--build-info"))
        assert metadata["platform"] == "linux/amd64"
        assert metadata["version"] in self.cli("--version")
        self.cli("--help")
        for command in ("check", "init", "performance"):
            self.cli(command, "--help")
        assert "modelcontextprotocol/go-sdk" in self.cli("--licenses")
        self.cli("init", "-n", "--php-runtime=8.3", "--agents=all", "--hooks=none")
        config = (self.project / ".phpwarden.yaml").read_bytes()
        self.cli("init", "-n", "--agents=all", "--hooks=none")
        assert config == (self.project / ".phpwarden.yaml").read_bytes()
        self.run(["git", "add", "--all"])
        self.run(
            [
                "git",
                "-c",
                "user.name=Fixture",
                "-c",
                "user.email=fixture@example.invalid",
                "-c",
                "commit.gpgsign=false",
                "commit",
                "-qm",
                "Initialized offline fixture",
            ]
        )
        self.summary["installation"] = {
            "installed": True,
            "reinstalled": True,
            "bad_checksum_preserved_binary": True,
            "init_preserved_config": True,
        }

    def missing_docker(self):
        self.summary["stage"] = "missing Docker diagnostics"
        args = ("check", "--mode=full", "--only=syntax", "--no-cache")
        report = json.loads(self.cli(*args, "--format=json", expected=(3,)))
        assert not report["complete"] and report["exit_code"] == 3
        assert not report["findings"]
        assert "docker" in json.dumps(report).lower()
        assert any(
            row["code"] == "docker_unavailable" and "install Docker" in row["message"]
            for row in report["diagnostics"]
        )
        assert not report.get("checks"), report
        text = self.cli(*args, expected=(3,))
        assert "NOT RUN" in text and "exit 3" in text and "Docker" in text
        (self.root / "missing-docker.json").write_text(json.dumps(report, indent=2))
        (self.root / "missing-docker.txt").write_text(text)
        self.summary["missing_docker"] = {"exit_code": 3, "complete": False, "checks_run": 0}

    def reader_arguments(self, reports=None):
        return [
            "--reports",
            reports or self.reports,
            "--producer",
            self.producer,
            "--project",
            self.project,
            "--container-root=/fixture",
        ]

    def performance(self):
        self.summary["stage"] = "recorded profiles with empty PATH"
        print("Docker-less: reading recorded profiles through CLI and five MCP tools", flush=True)
        self.environment["PATH"] = ""
        args = self.reader_arguments()
        report = json.loads(self.cli("performance", *args, "--format=json"))
        assert report["complete"] and len(report["profiles"]) == 2
        for profile in report["profiles"]:
            expected = 24 if profile["key"] == "repeated" else 1
            function = next(
                row
                for row in profile["top_functions"]
                if row["name"] == "phpwarden_expensive_lookup"
            )
            assert function["calls"] == expected and function["source_mapped"]
            assert function["file"] == "scenario.php" and function["definition_line"] == 2
        (self.root / "performance.json").write_text(json.dumps(report, indent=2))
        (self.root / "performance.txt").write_text(self.cli("performance", *args))
        with Client(
            [str(self.binary), "performance", "mcp", *map(str, args)],
            env=self.environment,
            cwd=self.project,
        ) as client:
            assert {row["name"] for row in client.request("tools/list")["tools"]} == TOOLS
            assert client.call("find_reports", {})["total"] == 2
            for key, calls in (("repeated", 24), ("cached", 1)):
                query = {"report_key": key}
                metadata = client.call("get_report_metadata", query)
                raw = json.loads((self.reports / (key + ".json")).read_text())
                profile = metadata["profiles"][0]
                assert profile["calls"] == raw["call_count"]
                assert abs(profile["wall_ms"] - raw["wall_time_ms"] / 1000) < 0.002
                query["snapshot"] = metadata["snapshot"]
                function = client.call(
                    "get_flat_profile", dict(query, function_contains="phpwarden_expensive_lookup")
                )["functions"][0]
                assert function["calls"] == calls and function["source_mapped"]
                assert function["file"] == "scenario.php" and function["definition_line"] == 2
                callers = client.call("get_callers", dict(query, function_id=function["id"]))
                assert any(
                    row["name"] == "phpwarden_render_item" and row["calls"] == calls
                    for row in callers["callers"]
                )
                parent = next(
                    row for row in callers["callers"] if row["name"] == "phpwarden_render_item"
                )
                graph = client.call(
                    "get_aggregated_call_graph", dict(query, function_id=parent["id"], depth=1)
                )
                assert any(
                    row["callee_id"] == function["id"]
                    and row["caller_id"] == parent["id"]
                    and row["calls"] == calls
                    for row in graph["edges"]
                )
            traversal = client.request(
                "tools/call",
                {"name": "get_report_metadata", "arguments": {"report_key": "../../etc/passwd"}},
            )
            assert traversal.get("isError")
        transcript = json.dumps(client.messages, indent=2)
        (self.root / "mcp-transcript.json").write_text(transcript)
        self.summary.update(profiles=2, mcp_tools=sorted(TOOLS), empty_path_reader=True)

    def incomplete(self):
        negative = self.root / "negative"
        negative.mkdir()
        shutil.copyfile(self.reports / "repeated.json", negative / "repeated.json")
        body = gzip.decompress((self.reports / "repeated.txt.gz").read_bytes())
        (negative / "repeated.txt.gz").write_bytes(gzip.compress(body[: len(body) // 2]))
        args = self.reader_arguments(negative)
        report = json.loads(self.cli("performance", *args, "--format=json", expected=(3,)))
        assert not report["complete"] and not report["profiles"]
        with Client(
            [str(self.binary), "performance", "mcp", *map(str, args)],
            env=self.environment,
            cwd=self.project,
        ) as client:
            result = client.call("get_flat_profile", {"report_key": "repeated"}, complete=False)
            assert result["exit_code"] == 3 and not result.get("functions")
        (self.root / "incomplete-profile.json").write_text(json.dumps(report, indent=2))
        self.summary["truncated_profile_rejected"] = True

    def execute(self):
        try:
            self.prepare()
            self.install()
            project, reports = fingerprint(self.project), fingerprint(self.reports)
            self.missing_docker()
            self.performance()
            self.incomplete()
            for name in ("performance.json", "performance.txt", "mcp-transcript.json"):
                text = (self.root / name).read_text()
                assert all(
                    value not in text
                    for value in (
                        "dockerless-cli-secret",
                        "dockerless-private-host",
                        "dockerless-private-metadata",
                    )
                )
            assert project == fingerprint(self.project) and reports == fingerprint(self.reports)
            self.environment["PATH"] = str(self.root / "helpers")
            assert not self.run(["git", "status", "--porcelain=v1", "--untracked-files=all"])
            self.summary.update(
                passed=True,
                project_preserved=True,
                profiles_preserved=True,
                redaction_verified=True,
                git_clean=True,
            )
        finally:
            (self.root / "summary.json").write_text(json.dumps(self.summary, indent=2))


def main():
    parser = argparse.ArgumentParser(
        description="Offline CLI and recorded-profile sandbox without Docker"
    )
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--archive", type=Path)
    args = parser.parse_args()
    directory = args.output.resolve()
    directory.mkdir(parents=True, exist_ok=False)
    archive = (
        args.archive.resolve() if args.archive else directory / "dist/phpwarden-linux-amd64.tar.gz"
    )
    if not args.archive:
        commit = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True, check=True
        ).stdout.strip()
        package("v0.0.0-dev." + commit[:7], commit, None, directory / "dist")
    sandbox = DockerlessSandbox(directory, archive)
    sandbox.execute()
    print(json.dumps(sandbox.summary, indent=2), flush=True)


if __name__ == "__main__":
    main()
