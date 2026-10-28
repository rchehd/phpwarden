import argparse
import gzip
import hashlib
import json
import os
import secrets
import shutil
import statistics
import subprocess
import sys
import time
import urllib.request
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.distribution import package  # noqa: E402
from tools.sandbox.commands import Commands  # noqa: E402
from tools.sandbox.compose import ComposeInstallation  # noqa: E402
from tools.sandbox.ddev import DdevRuntime  # noqa: E402
from tools.sandbox.docker import DockerRuntime  # noqa: E402
from tools.sandbox.host import HostRuntime  # noqa: E402
from tools.sandbox.mcp import Client  # noqa: E402
from tools.sandbox.packages import PackageInstallations  # noqa: E402

WORKER = "phpwarden:static-php8.3-dev"


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


class Sandbox(Commands):
    def __init__(
        self,
        major,
        directory,
        worker,
        archive,
        repeats,
        installation="standalone",
        environment="compose",
        host_php="php",
        host_extensions=None,
    ):
        self.major, self.root = major, directory
        self.project, self.logs = directory / "project", directory / "logs"
        self.logs.mkdir(parents=True)
        self.project.mkdir()
        self.name = "phpwarden-sandbox-" + uuid.uuid4().hex[:12]
        self.image = self.name + ":application"
        self.worker, self.archive, self.repeats = worker, archive, repeats
        self.installation = installation
        self.repository = ROOT
        self.user = f"{os.getuid()}:{os.getgid()}"
        self.environment = os.environ | {
            "XDG_CACHE_HOME": str(directory / "cache"),
            "CLAUDE_CONFIG_DIR": str(directory / "claude"),
            "npm_config_cache": str(directory / "npm-cache"),
        }
        (directory / "claude").mkdir()
        self.binary = directory / "bin/phpwarden"
        self.alias = False
        self.precommit_contents = None
        self.sequence = 0
        self.summary = {
            "drupal_major": major,
            "environment": environment,
            "installation_selection": installation,
            "passed": False,
        }
        self.container_root = "/project"
        self.ini = "/usr/local/etc/php/conf.d/99-phpwarden-spx.ini"
        self.http_url = "http://127.0.0.1:8080/phpwarden-probe"
        self.runtime = DdevRuntime(self) if environment == "ddev" else None
        if environment == "docker":
            self.runtime = DockerRuntime(self)
        if environment == "host":
            self.runtime = HostRuntime(self, host_php, host_extensions)

    def stage(self, message):
        self.summary["stage"] = message
        print(f"Drupal {self.major}: {message}", flush=True)

    def compose(self, *args):
        return self.run(
            [
                "docker",
                "compose",
                "--project-directory",
                self.project,
                "-f",
                self.root / "compose.json",
                "-p",
                self.name,
                *args,
            ]
        )

    def exec(self, args, profile=False):
        if isinstance(self.runtime, HostRuntime):
            return self.runtime.exec(args, profile)
        command = ["docker", "exec"]
        if self.runtime:
            command += ["--user", self.user]
        if profile:
            for entry in [
                "SPX_ENABLED=1",
                "SPX_REPORT=full",
                "SPX_METRICS=wt,ct,zm",
                "SPX_SAMPLING_PERIOD=0",
            ]:
                command += ["--env", entry]
        mapped = [str(arg).replace("/project", self.container_root) for arg in args]
        return self.run([*command, self.container, "php", *mapped])

    def drush(self, *args, profile=False):
        return self.exec(
            [
                "/project/vendor/drush/drush/drush.php",
                "--root=/project/web",
                "--uri=http://localhost",
                *args,
            ],
            profile,
        )

    def git_status(self):
        return self.run(["git", "status", "--porcelain=v1", "-z", "--untracked-files=all"])

    def install(self):
        self.stage("preparing matching quality worker")
        self.worker_id = self.run(
            ["docker", "image", "inspect", self.worker, "--format", "{{.Id}}"]
        ).strip()
        alias = subprocess.run(
            ["docker", "image", "inspect", WORKER, "--format", "{{.Id}}"],
            capture_output=True,
            text=True,
        )
        if alias.returncode:
            self.run(["docker", "tag", self.worker_id, WORKER])
            self.alias = True
        elif alias.stdout.strip() != self.worker_id:
            raise RuntimeError(
                "Sandbox worker differs from the existing development tag; refusing to replace it"
            )
        if isinstance(self.runtime, HostRuntime):
            self.runtime.install_project()
            return
        self.stage("building isolated application image and installing locked dependencies")
        self.run(
            [
                "docker",
                "build",
                "--pull=false",
                "--build-arg",
                f"WORKER_IMAGE={self.worker}",
                "--tag",
                self.image,
                ROOT / "tools/sandbox",
            ],
            timeout=600,
        )
        fixture = ROOT / f"tools/sandbox/drupal{self.major}"
        for name in ["composer.json", "composer.lock"]:
            shutil.copyfile(fixture / name, self.project / name)
        assert not json.loads((self.project / "composer.json").read_text()).get("scripts")
        command = [
            "docker",
            "run",
            "--rm",
            "--name",
            self.name + "-install",
            "--user",
            f"{os.getuid()}:{os.getgid()}",
            "--mount",
            f"type=bind,src={self.project},dst=/project",
            "--workdir",
            "/project",
            "--env",
            "COMPOSER_HOME=/tmp/composer-home",
            "--env",
            "COMPOSER_CACHE_DIR=/tmp/composer-cache",
            "--entrypoint",
            "php",
            self.image,
        ]
        self.run(
            [
                *command,
                "/usr/local/lib/phpwarden/composer/vendor/bin/composer",
                "install",
                "--no-dev",
                "--no-interaction",
                "--prefer-dist",
            ],
            timeout=600,
        )
        module = self.project / "web/modules/custom/phpwarden_probe"
        shutil.copytree(ROOT / "tools/sandbox/fixture", module)
        public = self.project / "web/sites/default/files"
        public.mkdir(parents=True, exist_ok=True)
        (self.root / "data").mkdir()
        settings = self.project / "web/sites/default/settings.php"
        settings.write_text(
            "<?php\n$settings['hash_salt'] = '"
            + secrets.token_hex(32)
            + "';\n$settings['trusted_host_patterns'] = "
            + "['^localhost$', '^127\\\\.0\\\\.0\\\\.1$'];\n"
            + "$settings['config_sync_directory'] = '/data/config-sync';\n"
        )
        self.run(
            [
                *command[:-1],
                "--network",
                "none",
                "--mount",
                f"type=bind,src={self.root / 'data'},dst=/data",
                command[-1],
                "/project/vendor/drush/drush/drush.php",
                "--root=/project/web",
                "--uri=http://localhost",
                "site:install",
                "minimal",
                "--yes",
                "--db-url=sqlite://localhost//data/site.sqlite?module=sqlite",
                "--account-pass=fixture-local-only",
            ],
            timeout=240,
        )
        self.run(
            [
                *command[:-1],
                "--network",
                "none",
                "--mount",
                f"type=bind,src={self.root / 'data'},dst=/data",
                command[-1],
                "/project/vendor/drush/drush/drush.php",
                "--root=/project/web",
                "--uri=http://localhost",
                "pm:enable",
                "phpwarden_probe",
                "--yes",
            ]
        )
        compose = {
            "services": {
                "php": {
                    "image": self.image,
                    "entrypoint": ["php"],
                    "command": [
                        "-S",
                        "127.0.0.1:8080",
                        "-t",
                        "/project/web",
                        "/project/web/index.php",
                    ],
                    "working_dir": "/project/web",
                    "user": f"{os.getuid()}:{os.getgid()}",
                    "network_mode": "none",
                    "read_only": True,
                    "cap_drop": ["ALL"],
                    "security_opt": ["no-new-privileges:true"],
                    "tmpfs": ["/tmp:rw,nosuid,nodev,size=256m"],
                    "volumes": [
                        f"{self.project}:/project:ro",
                        f"{self.root / 'data'}:/data",
                        f"{public}:/project/web/sites/default/files",
                    ],
                }
            }
        }
        (self.root / "compose.json").write_text(json.dumps(compose, indent=2))
        self.finish_install()

    def finish_install(self):
        (self.project / ".gitignore").write_text(
            "/vendor/\n/node_modules/\n/web/core/\n/web/sites/default/files/\n"
            "/web/sites/default/settings.php\n"
        )
        self.run(["git", "init", "-q"])
        self.run(["git", "add", "--all"])
        self.run(
            [
                "git",
                "-c",
                "user.name=Fixture",
                "-c",
                "user.email=fixture@example.invalid",
                "commit",
                "-qm",
                "Installed Drupal fixture",
            ]
        )

    def standalone_checks(self):
        self.stage("checking standalone install and reinstall")
        self.run(
            ["sh", ROOT / "install.sh", "--archive", self.archive, "--dir", self.binary.parent]
        )
        self.run(
            [
                "sh",
                ROOT / "install.sh",
                "--archive",
                self.archive,
                "--dir",
                self.binary.parent,
                "--force",
            ]
        )
        self.project_checks("standalone")
        self.summary.setdefault("installations", {})["standalone"] = {
            "installed": True,
            "reinstalled": True,
        }

    def project_checks(self, method, completion=True, require_fix=False):
        self.stage(f"checking {method} init, fix, static checks and hooks")
        hook = self.project / ".git/hooks/pre-commit"
        if self.precommit_contents is not None:
            assert hook.read_bytes() == self.precommit_contents
            hook.unlink()
        assert "modelcontextprotocol/go-sdk" in self.cli("--licenses")
        hooks = "all" if completion else "none"
        runtime = (
            ["--php-runtime=8.3"]
            if (self.runtime is None or isinstance(self.runtime, HostRuntime))
            else []
        )
        self.cli("init", "-n", *runtime, "--agents=all", "--hooks=" + hooks, "--pre-commit")
        config = self.project / ".phpwarden.yaml"
        initial = config.read_bytes()
        self.cli("init", "-n", "--agents=all", "--hooks=" + hooks, "--pre-commit")
        assert initial == config.read_bytes()
        effective = json.loads(self.cli("config", "show", "--format=json"))["effective"]["config"]
        assert effective["framework"]["version"].startswith(str(self.major) + ".")
        assert effective["php"]["runtime"] == "8.3"
        if completion:
            self.cli("doctor", "--format=json", expected=(0, 3))
        checks = (
            "syntax,debug,whitespace,yaml,phpcs,phpstan,dead-code,eventhorizon,"
            "rector,semgrep,jscpd,phpmd,drupal-config,composer-validate"
        )
        result = json.loads(
            self.cli("check", "--mode=full", "--only=" + checks, "--format=json", expected=(0, 1))
        )
        assert result["complete"], result.get("diagnostics")
        source = (
            self.project / "web/modules/custom/phpwarden_probe/src/Controller/ProbeController.php"
        )
        fixed = json.loads(
            self.cli(
                "fix",
                str(source.relative_to(self.project)),
                "--only=phpcs",
                "--format=json",
                expected=(0, 1),
            )
        )
        assert fixed["complete"], fixed
        if require_fix:
            assert source.relative_to(self.project).as_posix() in fixed["fixed_files"], fixed
            assert "$size = 0;" in source.read_text()
        original = source.read_bytes()
        simple = {
            "version": 1,
            "profile": "drupal",
            "php": effective["php"],
            "scope": {"base": "HEAD"},
            "tools": {
                name: {"enabled": name in ["debug", "syntax"]} for name in effective["tools"]
            },
        }
        config.write_text(json.dumps(simple))
        source.write_text("<?php\nvar_dump('sandbox-blocking-fixture');\n")
        for client in ["codex", "cursor", "claude"] if completion else []:
            event = {
                "hook_event_name": "stop" if client == "cursor" else "Stop",
                "session_id": self.name + "-" + method,
                "cwd": str(self.project),
            }
            decision = json.loads(
                self.cli("hook", "stop", "--client=" + client, input=json.dumps(event))
            )
            assert (
                decision.get("followup_message")
                if client == "cursor"
                else decision.get("decision") == "block"
            ), decision
            assert "var_dump" in json.dumps(decision), decision
        self.run(["git", "add", "web/modules/custom"])
        self.run([self.project / ".git/hooks/pre-commit"], expected=(1,))
        source.write_bytes(original)
        self.run(["git", "add", "web/modules/custom"])
        self.run([self.project / ".git/hooks/pre-commit"])
        config.write_bytes(initial)
        self.checkpoint(f"Verified {method} installation")
        self.precommit_contents = hook.read_bytes()

    def checkpoint(self, message):
        if not self.git_status():
            return
        self.run(["git", "add", "--all"])
        self.run(
            [
                "git",
                "-c",
                "core.hooksPath=/dev/null",
                "-c",
                "user.name=Fixture",
                "-c",
                "user.email=fixture@example.invalid",
                "commit",
                "-qm",
                message,
            ]
        )
        assert self.git_status() == ""

    def lifecycle(self, action, *args, expected=(0,)):
        return json.loads(
            self.cli(
                "performance",
                action,
                "--project",
                self.project,
                "--state-dir",
                self.root / "state",
                "--format=json",
                *args,
                expected=expected,
                timeout=960,
            )
        )

    def http(self, mode="on"):
        code = (
            """$headers="";
if ("MODE" !== "off") {
  $ini=parse_ini_file("INI_PATH");
  $key="MODE" === "bad" ? "wrong-fixture-key" : $ini["spx.http_key"];
  $headers="Cookie: SPX_ENABLED=1; SPX_KEY=".$key."; SPX_REPORT=full; ";
  $headers.="SPX_METRICS=wt,ct,zm; SPX_SAMPLING_PERIOD=0\\r\\n";
}
$context=stream_context_create(["http"=>["header"=>$headers,"ignore_errors"=>true,"timeout"=>45]]);
$body=file_get_contents("HTTP_URL?token=fixture-query-secret",false,$context);
$decoded=json_decode($body,true);
$preview=$decoded === null ? substr($body,0,2000) : null;
if (!empty($key) && $preview !== null) { $preview=str_replace($key,"[redacted]",$preview); }
echo json_encode(["status"=>$http_response_header[0],"body"=>$decoded,
  "preview"=>$preview]);""".replace("MODE", mode)
            .replace("INI_PATH", self.ini)
            .replace("HTTP_URL", self.http_url)
        )
        result = json.loads(self.exec(["-r", code]))
        if mode == "bad":
            assert "200" in result["status"], result
            return result
        assert (
            "200" in result["status"]
            and isinstance(result["body"], dict)
            and result["body"]["config_rows"] == 5
        ), result
        assert result["body"]["driver"] == "sqlite"
        return result["body"]

    def current_container(self):
        return (
            self.runtime.current_container()
            if self.runtime
            else self.compose("ps", "-q", "php").strip()
        )

    def setup_arguments(self):
        if self.runtime:
            return self.runtime.setup_arguments()
        return [
            "--environment=compose",
            "--php=8.3",
            "--service=php",
            "--compose-file",
            self.root / "compose.json",
            "--compose-project",
            self.name,
            "--container-root=/project",
        ]

    def start_runtime(self):
        if self.runtime:
            self.runtime.start()
        else:
            self.compose("up", "-d", "--no-build", "--pull", "never", "php")

    def capture(self):
        self.stage("recording database-backed HTTP/Drush cold and warm profiles")
        self.start_runtime()
        setup = self.lifecycle(
            "setup",
            *self.setup_arguments(),
            "--http",
            "--spx-archive",
            self.root / "spx.tar.gz",
            "--apply",
        )
        if self.runtime and hasattr(self.runtime, "activate"):
            setup = self.runtime.activate(setup)
        assert setup["loaded"] and setup["state"] == "active"
        self.container = self.current_container()
        retained = json.loads(
            self.exec(
                [
                    "-r",
                    'echo json_encode(["sha256"=>hash_file("sha256",'
                    '"/opt/phpwarden-spx/source.tar.gz"),'
                    '"license"=>is_file("/opt/phpwarden-spx/LICENSE"),'
                    '"build"=>is_file("/opt/phpwarden-spx/build.sh")]);',
                ]
            )
        )
        assert retained == {"sha256": self.pin["sha256"], "license": True, "build": True}
        self.reports = Path(setup["reports"])
        self.http_key = Path(setup["http_key_file"]).read_text().strip()

        def keys():
            return {p.stem for p in self.reports.glob("*.json")}

        before = keys()
        self.http("off")
        self.http("bad")
        assert keys() == before
        self.captures = []
        for kind in ["http", "drush"]:
            for index in range(self.repeats):
                self.drush("cache:rebuild")
                for temperature in ["cold", "warm"]:
                    before = keys()
                    if kind == "http":
                        self.http()
                    else:
                        result = json.loads(
                            self.drush(
                                "php:eval",
                                '$marker="fixture-cli-secret"; echo '
                                "\\Drupal\\phpwarden_probe\\Controller\\ProbeController::page()"
                                "->getContent();",
                                profile=True,
                            )
                        )
                        assert result["config_rows"] == 5
                    for _ in range(30):
                        added = keys() - before
                        if added:
                            break
                        time.sleep(0.1)
                    assert len(added) == 1, added
                    self.captures.append(
                        {
                            "scenario": f"{kind}-{temperature}",
                            "iteration": index,
                            "key": added.pop(),
                        }
                    )
        (self.root / "captures.json").write_text(json.dumps(self.captures, indent=2))

    def verify_mcp(self):
        self.stage(
            "validating five MCP tools, redaction, graph, source mapping and malformed captures"
        )
        self.lifecycle("connect", "--client=all")
        repeated = self.lifecycle("connect", "--client=all")
        assert not any(c["changed"] for c in repeated["connections"])
        server = json.loads((self.project / ".cursor/mcp.json").read_text())["mcpServers"][
            "phpwarden-performance"
        ]
        profiles = []
        with Client([server["command"], *server["args"]]) as client:
            assert len(client.request("tools/list")["tools"]) == 5
            assert client.call("find_reports", {})["total"] == len(self.captures)
            for capture in self.captures:
                query = {"report_key": capture["key"]}
                metadata = client.call("get_report_metadata", query)
                raw = json.loads((self.reports / (capture["key"] + ".json")).read_text())
                profile = metadata["profiles"][0]
                assert profile["calls"] == raw["call_count"] and profile["peak_memory_bytes"] > 0
                assert abs(profile["wall_ms"] - raw["wall_time_ms"] / 1000) < 0.002
                profiles.append(dict(capture, **profile))
                if capture["iteration"] != 0 or capture["scenario"].endswith("warm"):
                    continue
                query["snapshot"] = metadata["snapshot"]
                flat = client.call(
                    "get_flat_profile", dict(query, function_contains="ProbeController::hotspot")
                )
                assert flat["total"] == 1
                function = flat["functions"][0]
                assert function["calls"] == 1 and function["source_mapped"]
                path = "web/modules/custom/phpwarden_probe/src/Controller/ProbeController.php"
                assert function["file"] == path
                assert (
                    "function hotspot"
                    in (self.project / path)
                    .read_text()
                    .splitlines()[function["definition_line"] - 1]
                )
                callers = client.call("get_callers", dict(query, function_id=function["id"]))
                assert any(
                    r["name"].endswith("ProbeController::page") and r["calls"] == 1
                    for r in callers["callers"]
                )
                graph = client.call(
                    "get_aggregated_call_graph", dict(query, function_id=function["id"], depth=1)
                )
                yaml = next(
                    r
                    for r in graph["functions"]
                    if r["name"] == "Drupal\\Component\\Serialization\\Yaml::encode"
                )
                assert any(
                    e["callee_id"] == yaml["id"]
                    and e["caller_id"] == function["id"]
                    and e["calls"] == 40
                    for e in graph["edges"]
                )
            assert client.request(
                "tools/call",
                {"name": "get_report_metadata", "arguments": {"report_key": "../../etc/passwd"}},
            ).get("isError")
        transcript = json.dumps(client.messages, indent=2)
        assert all(
            secret not in transcript
            for secret in [self.http_key, "fixture-cli-secret", "fixture-query-secret"]
        )
        (self.root / "mcp-transcript.json").write_text(transcript)
        negative = self.root / "negative"
        negative.mkdir()
        key = self.captures[0]["key"]
        shutil.copyfile(self.reports / (key + ".json"), negative / (key + ".json"))
        body = gzip.decompress((self.reports / (key + ".txt.gz")).read_bytes())
        (negative / (key + ".txt.gz")).write_bytes(
            gzip.compress(b"\n".join(body.splitlines()[:-1]) + b"\n")
        )
        (negative / "malformed.json").write_text("{broken")
        with Client(
            [
                self.binary,
                "performance",
                "mcp",
                "--reports",
                negative,
                "--producer",
                self.pin["commit"],
            ]
        ) as bad:
            bad.call("find_reports", {}, complete=False)
            bad.call("get_report_metadata", {"report_key": key}, complete=False)
        self.summary["profiles"] = len(profiles)
        self.summary["wall_median_ms"] = {
            scenario: statistics.median(r["wall_ms"] for r in profiles if r["scenario"] == scenario)
            for scenario in ["http-cold", "http-warm", "drush-cold", "drush-warm"]
        }

    def teardown(self):
        self.stage("checking teardown, preserved profiles and unchanged Git")
        hashes = {p.name: sha(p) for p in self.reports.iterdir()}
        self.lifecycle("disconnect", "--client=all")
        arguments = ["--apply"]
        if self.runtime and hasattr(self.runtime, "restore"):
            self.runtime.restore()
            arguments = []
        assert self.lifecycle("teardown", *arguments)["state"] == "removed"
        assert self.lifecycle("teardown", *arguments)["state"] == "removed"
        self.container = self.current_container()
        assert self.exec(["-r", 'echo extension_loaded("spx")?"yes":"no";']) == "no"
        self.http("off")
        assert hashes == {p.name: sha(p) for p in self.reports.iterdir()}
        result = json.loads(
            self.cli(
                "performance",
                "--reports",
                self.reports,
                "--producer",
                self.pin["commit"],
                "--format=json",
            )
        )
        assert result["complete"] and len(result["profiles"]) == len(self.captures)
        assert self.git_status() == ""
        self.summary.update(passed=True, git_clean=True, spx_unloaded=True, profiles_preserved=True)

    def execute(self):
        try:
            if not isinstance(self.runtime, HostRuntime):
                self.pin = json.loads((ROOT / "tools/spx/sources.json").read_text())["spx"]
                with urllib.request.urlopen(self.pin["url"], timeout=30) as response:
                    archive = response.read((2 << 20) + 1)
                assert (
                    len(archive) <= 2 << 20
                    and hashlib.sha256(archive).hexdigest() == self.pin["sha256"]
                )
                (self.root / "spx.tar.gz").write_bytes(archive)
            self.install()
            if self.runtime:
                self.runtime.install()
            if self.installation in ("standalone", "all"):
                self.standalone_checks()
            elif self.installation == "compose":
                self.run(
                    [
                        "sh",
                        ROOT / "install.sh",
                        "--archive",
                        self.archive,
                        "--dir",
                        self.binary.parent,
                    ]
                )
            if self.installation in ("composer", "npm", "all"):
                packages = PackageInstallations(self)
                for manager in ("composer", "npm"):
                    if self.installation in (manager, "all"):
                        packages.run(manager)
            if self.installation in ("compose", "all"):
                ComposeInstallation(self).run()
            if not isinstance(self.runtime, HostRuntime):
                self.capture()
                self.verify_mcp()
                self.teardown()
            if self.runtime:
                self.runtime.preserved()
        except Exception:
            self.summary["passed"] = False
            raise
        finally:
            errors = []
            try:
                subprocess.run(
                    ["docker", "rm", "--force", self.name + "-install"],
                    capture_output=True,
                    timeout=30,
                )
            except (OSError, subprocess.TimeoutExpired) as error:
                errors.append(str(error))
            try:
                if self.runtime:
                    self.runtime.cleanup()
            except Exception as error:
                errors.append(str(error))
            try:
                if self.runtime is None and (self.root / "compose.json").exists():
                    try:
                        if not self.summary["passed"]:
                            self.compose("logs", "--no-color", "--tail=100", "php")
                    finally:
                        self.compose("down", "--remove-orphans")
            except Exception as error:
                errors.append(str(error))
            try:
                if self.alias:
                    current = self.run(
                        ["docker", "image", "inspect", WORKER, "--format", "{{.Id}}"]
                    ).strip()
                    if current == self.worker_id:
                        self.run(["docker", "image", "rm", WORKER])
            except Exception as error:
                errors.append(str(error))
            if errors:
                self.summary.update(passed=False, cleanup_errors=errors)
            (self.root / "summary.json").write_text(json.dumps(self.summary, indent=2))
            if errors:
                raise RuntimeError("Sandbox cleanup failed; see summary.json")


def main():
    parser = argparse.ArgumentParser(
        description="Install and test an isolated Drupal database sandbox"
    )
    parser.add_argument("--drupal", choices=("10", "11"), required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--worker-image", default=WORKER)
    parser.add_argument("--archive", type=Path)
    parser.add_argument(
        "--environment", choices=("compose", "ddev", "docker", "host"), default="compose"
    )
    parser.add_argument("--host-php", default="php", help="PHP 8.3 executable for the host sandbox")
    parser.add_argument(
        "--host-extensions", type=Path, help="Additional native PHP extension directory"
    )
    parser.add_argument(
        "--installation", choices=("standalone", "composer", "npm", "compose", "all")
    )
    parser.add_argument("--repeats", type=int, choices=range(1, 4), default=3)
    args = parser.parse_args()
    installation = args.installation or ("standalone" if args.archive else "all")
    if args.archive and installation != "standalone":
        parser.error(
            "--archive selects standalone installation; other candidates are built from source"
        )
    directory = args.output.resolve()
    directory.mkdir(parents=True, exist_ok=False)
    archive = (
        args.archive.resolve() if args.archive else directory / "dist/phpwarden-linux-amd64.tar.gz"
    )
    if not args.archive and installation in ("standalone", "compose", "all"):
        commit = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True, check=True
        ).stdout.strip()
        package("v0.0.0-dev." + commit[:7], commit, None, directory / "dist")
    sandbox = Sandbox(
        args.drupal,
        directory,
        args.worker_image,
        archive,
        args.repeats,
        installation,
        args.environment,
        args.host_php,
        args.host_extensions,
    )
    sandbox.execute()
    print(json.dumps(sandbox.summary, indent=2), flush=True)


if __name__ == "__main__":
    main()
