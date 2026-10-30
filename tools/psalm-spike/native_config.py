import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import time
import uuid
import xml.etree.ElementTree as ET
from pathlib import Path
from types import SimpleNamespace

from run import SHA256, VERSION

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
from tools.psalm.prepare import installed_packages, read  # noqa: E402
from tools.sandbox.workspace import finish  # noqa: E402


def docker(args, project, private, entrypoint, *, writable=False):
    command = [
        "docker",
        "run",
        "--pull=never",
        "--rm",
        "--name",
        "phpwarden-psalm-native-" + uuid.uuid4().hex[:12],
        "--cpus=0.2",
        "--network=none",
        "--read-only",
        "--user",
        f"{os.getuid()}:{os.getgid()}",
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges",
        "--pids-limit=128",
        "--memory=1g",
        "--tmpfs",
        "/tmp:rw,nosuid,size=256m,mode=1777",
        "--mount",
        f"type=bind,src={project},dst=/project" + ("" if writable else ",readonly"),
        "--mount",
        f"type=bind,src={private},dst=/private",
        "--mount",
        f"type=bind,src={args.binary},dst=/opt/native-config,readonly",
    ]
    if args.bundle:
        command += ["--mount", f"type=bind,src={args.bundle / 'vendor'},dst=/schema,readonly"]
    else:
        command += [
            "--mount",
            f"type=bind,src={args.phar},dst=/opt/psalm.phar,readonly",
            "--mount",
            f"type=bind,src={args.schema},dst=/schema/vimeo/psalm/config.xsd,readonly",
        ]
    return command + [
        "--workdir=/project",
        "--entrypoint",
        entrypoint,
        args.image,
    ]


def engine(args):
    if args.bundle:
        if args.schema:
            raise ValueError("Bundle mode uses its own pinned schema; omit --schema")
        args.bundle = args.bundle.resolve(strict=True)
        lock, packages = installed_packages(args.bundle)
        for name in ("autoload.php", "bin/psalm", "vimeo/psalm/config.xsd"):
            read(args.bundle / "vendor" / name)
        return "/schema/bin/psalm", {
            "engine": "composer",
            "bundle_lock_sha256": hashlib.sha256(lock).hexdigest(),
            "packages": len(packages),
        }
    if not args.schema:
        raise ValueError("PHAR mode requires --schema")
    args.phar = args.phar.resolve(strict=True)
    args.schema = args.schema.resolve(strict=True)
    if hashlib.sha256(read(args.phar)).hexdigest() != SHA256:
        raise ValueError("Pinned Psalm PHAR checksum mismatch")
    return "/opt/psalm.phar", {"engine": "phar", "phar_sha256": SHA256}


def bundle_snapshot(bundle):
    result = {}
    for path in sorted(bundle.rglob("*")):
        if path.is_symlink():
            raise ValueError("Native bundle acceptance requires regular files and directories")
        if path.is_file():
            result[path.relative_to(bundle).as_posix()] = hashlib.sha256(read(path)).hexdigest()
    return result


def verify_bundle(bundle, before, summary):
    passed = summary["passed"]
    summary["passed"] = False
    summary["bundle_preserved"] = False
    summary["bundle_preserved"] = bundle_snapshot(bundle) == before
    if not summary["bundle_preserved"]:
        raise RuntimeError("Psalm bundle inputs changed during acceptance")
    summary["passed"] = passed


def execute(command, output, name):
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=180)
    finally:
        container = command[command.index("--name") + 1]
        subprocess.run(["docker", "rm", "--force", container], capture_output=True, timeout=20)
    (output / f"{name}.stdout").write_text(result.stdout)
    (output / f"{name}.stderr").write_text(result.stderr)
    return result


def runtime_details(result, expected=None):
    if result.returncode != 0 or result.stderr:
        raise ValueError("PHP runtime probe failed")
    value = json.loads(result.stdout)
    if not isinstance(value, dict):
        raise ValueError("PHP runtime probe did not return an object")
    version = value.get("version")
    match = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)", version) if isinstance(version, str) else None
    if not match:
        raise ValueError("PHP runtime probe did not return a stable patch version")
    major, minor, patch = map(int, match.groups())
    runtime = f"{major}.{minor}"
    if (
        type(value.get("id")) is not int
        or value["id"] != major * 10000 + minor * 100 + patch
        or value.get("runtime") != runtime
    ):
        raise ValueError("PHP runtime probe returned inconsistent version metadata")
    extensions = value.get("extensions")
    if (
        not isinstance(extensions, list)
        or not extensions
        or any(not isinstance(name, str) or not name for name in extensions)
        or len(set(extensions)) != len(extensions)
    ):
        raise ValueError("PHP runtime extension inventory is invalid")
    if expected and expected != runtime:
        raise ValueError(f"Expected PHP {expected}, actual runtime is {runtime}; no fallback")
    return {
        "version": version,
        "id": value["id"],
        "runtime": runtime,
        "extensions": sorted(extensions),
    }


def baseline(issues):
    root = ET.Element("files", {"psalm-version": VERSION})
    files = {}
    for issue in issues:
        path = issue["file_path"]
        name = path.removeprefix("/project/")
        if path == "/private/prepared/entrypoints.php":
            name = "@generated/entrypoints.php"
        if name not in files:
            files[name] = ET.SubElement(root, "file", {"src": name})
        file = files[name]
        kind = file.find(issue["type"])
        if kind is None:
            kind = ET.SubElement(file, issue["type"])
        ET.SubElement(kind, "code").text = issue["selected_text"]
    return ET.tostring(root, encoding="unicode")


def seed(project, kind, regular_baseline, generated_baseline):
    (project / "src").mkdir(parents=True)
    (project / "src/Helper.php").write_text(
        "<?php\nfunction getName(): string { return (string) ($_GET['name'] ?? ''); }\n"
    )
    entry = "echo getName();"
    generated = ""
    policy = ""
    if kind == "safe":
        entry = "echo htmlspecialchars(getName(), ENT_QUOTES, 'UTF-8');"
    if kind in {"generated", "generated-baseline"}:
        entry, generated = "", "echo getName();"
    if kind in {"baseline", "baseline-new", "generated-baseline"}:
        policy = '<psalm errorBaseline="taint.xml"/>'
        (project / "taint.xml").write_text(
            generated_baseline if kind == "generated-baseline" else regular_baseline
        )
    if kind == "baseline-new":
        entry += "\necho (string) ($_GET['other'] ?? '');"
    if kind == "handlers":
        policy = (
            '<psalm><issueHandlers><TaintedHtml errorLevel="suppress"/>'
            '<TaintedTextWithQuotes errorLevel="suppress"/></issueHandlers></psalm>'
        )
    if kind == "readiness":
        entry = "new MissingType();\n" + entry
        policy = (
            '<psalm errorLevel="6"><issueHandlers><TaintedHtml errorLevel="suppress"/>'
            "</issueHandlers></psalm>"
        )
    if kind == "stubs":
        entry = "echo externalInput();"
        (project / "types.php").write_text(
            "<?php\n/** @psalm-taint-source input */\nfunction externalInput(): string {}\n"
        )
        policy = '<psalm><stubs><file name="types.php"/></stubs></psalm>'
    if kind in {"php84-hook", "php84-hook-safe"}:
        expression = "(string) ($_GET['name'] ?? '')"
        if kind == "php84-hook-safe":
            expression = f"htmlspecialchars({expression}, ENT_QUOTES, 'UTF-8')"
        (project / "src/HookedInput.php").write_text(
            "<?php\nfinal class HookedInput {\n"
            f"public string $value {{ get => {expression}; }}\n}}\n"
        )
        entry = "require_once __DIR__ . '/HookedInput.php';\necho (new HookedInput())->value;"
    if kind == "schema":
        policy = (
            '<psalm><issueHandlers><TaintedNotAnUpstreamIssue errorLevel="suppress"/>'
            "</issueHandlers></psalm>"
        )
    if policy:
        (project / "psalm.xml").write_text(policy)
    (project / "src/Entry.php").write_text("<?php\n" + entry + "\n")
    (project / "generated.php").write_text("<?php\n" + generated + "\n")
    (project / "composer.json").write_text(
        json.dumps(
            {
                "name": "phpwarden/native-policy-fixture",
                "autoload": {"files": ["src/Helper.php"]},
            }
        )
    )
    (project / "profile-base.xml").write_text(
        '<psalm errorLevel="6" resolveFromConfigFile="true" cacheDirectory="/tmp/psalm-cache">'
        '<projectFiles><directory name="/project/src"/>'
        '<file name="/private/prepared/entrypoints.php"/></projectFiles></psalm>'
    )


def main():
    parser = argparse.ArgumentParser(
        description="Pinned native XML/baseline acceptance on disposable fixtures"
    )
    for name in ("binary", "output"):
        parser.add_argument("--" + name, type=Path, required=True)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--phar", type=Path)
    source.add_argument("--bundle", type=Path)
    parser.add_argument("--schema", type=Path)
    parser.add_argument("--image", required=True)
    parser.add_argument("--php", default="/usr/bin/php8.3")
    parser.add_argument(
        "--runtime",
        choices=("8.1", "8.2", "8.3", "8.4"),
        help="Require this actual PHP minor runtime; no fallback",
    )
    args = parser.parse_args()
    args.binary = args.binary.resolve(strict=True)
    try:
        entrypoint, identity = engine(args)
    except (OSError, ValueError, KeyError) as error:
        parser.error(str(error))
    tool_before = bundle_snapshot(args.bundle) if args.bundle else None
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    workspace = args.output / "workspace"
    workspace.mkdir()
    summary = identity | {"version": VERSION, "cases": [], "cpus": 0.2, "passed": False}
    regular_baseline = generated_baseline = ""
    try:
        probe = args.output / "runtime"
        project, private = workspace / "runtime/project", workspace / "runtime/private"
        probe.mkdir()
        project.mkdir(parents=True)
        private.mkdir()
        result = execute(
            docker(args, project, private, args.php)
            + [
                "-r",
                'echo json_encode(["version"=>PHP_VERSION,"id"=>PHP_VERSION_ID,'
                '"runtime"=>PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION,'
                '"extensions"=>get_loaded_extensions()], JSON_THROW_ON_ERROR);',
            ],
            probe,
            "probe",
        )
        summary["php"] = runtime_details(result, args.runtime)
        cases = [
            "default",
            "baseline",
            "baseline-new",
            "handlers",
            "readiness",
            "stubs",
            "generated",
            "generated-baseline",
            "safe",
            "schema",
        ]
        if summary["php"]["runtime"] == "8.4":
            cases += ["php84-hook", "php84-hook-safe"]
        for kind in cases:
            started = time.monotonic()
            output = args.output / kind
            output.mkdir()
            project, private = workspace / kind / "project", workspace / kind / "private"
            private.mkdir(parents=True)
            seed(project, kind, regular_baseline, generated_baseline)
            autoload = execute(
                docker(args, project, private, "composer", writable=True)
                + [
                    "--no-cache",
                    "dump-autoload",
                    "--no-scripts",
                    "--no-plugins",
                    "--no-interaction",
                ],
                output,
                "autoload",
            )
            if autoload.returncode:
                raise RuntimeError(f"Composer fixture preparation failed: {kind}")
            before = {
                str(p.relative_to(project)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in project.rglob("*")
                if p.is_file()
            }
            prepared = execute(
                docker(args, project, private, "/opt/native-config")
                + [
                    "--project=/project",
                    "--directory=/private",
                    "--bundle=/schema",
                    f"--php={args.php}",
                ],
                output,
                "prepare",
            )
            passed = False
            if kind == "schema":
                passed = prepared.returncode == 3 and "TaintedNotAnUpstreamIssue" in prepared.stderr
            elif prepared.returncode == 0:
                config = json.loads(prepared.stdout)
                command = docker(args, project, private, args.php) + [
                    entrypoint,
                    "--root=/project",
                    "--no-cache",
                    "--no-diff",
                    "--no-progress",
                    "--threads=1",
                    "--scan-threads=1",
                    "--show-info=false",
                    "--output-format=json",
                ]
                readiness = execute(
                    command + [f"--config={config['ReadinessConfig']}", "--ignore-baseline"],
                    output,
                    "readiness",
                )
                ready = json.loads(readiness.stdout)
                if kind == "readiness":
                    passed = readiness.returncode == 2 and any(
                        r["type"] == "UndefinedClass" for r in ready
                    )
                elif readiness.returncode == 0 and ready == [] and not readiness.stderr:
                    taint = execute(
                        command + [f"--config={config['Config']}", "--taint-analysis"],
                        output,
                        "taint",
                    )
                    issues = json.loads(taint.stdout)
                    expected = (
                        0
                        if kind
                        in {"baseline", "handlers", "generated-baseline", "safe", "php84-hook-safe"}
                        else 2
                    )
                    passed = (
                        len(issues) == expected
                        and taint.returncode == (2 if expected else 0)
                        and not taint.stderr
                    )
                    if kind == "default":
                        regular_baseline = baseline(issues)
                    if kind == "generated":
                        generated_baseline = baseline(issues)
            preserved = all(
                hashlib.sha256((project / p).read_bytes()).hexdigest() == digest
                for p, digest in before.items()
            )
            record = {
                "name": kind,
                "passed": passed and preserved,
                "project_preserved": preserved,
                "seconds": round(time.monotonic() - started, 2),
            }
            summary["cases"].append(record)
            print(json.dumps(record), flush=True)
            if not record["passed"]:
                raise RuntimeError(f"Native config acceptance failed: {kind}; see {output}")
        summary["passed"] = True
    finally:
        try:
            if args.bundle:
                verify_bundle(args.bundle, tool_before, summary)
        finally:
            try:
                finish(
                    SimpleNamespace(root=args.output, summary=summary, keep_workspace=False),
                    ("workspace",),
                    check_mounts=True,
                )
            finally:
                (args.output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
