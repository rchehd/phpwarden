import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PLUGIN_REF = "073e7a4d5fc159fbf47decf16296edd7f8a545d1"
MODULE = "phpwarden_taint_probe"
CONTROLS = {
    "trusted": ("src/Controller/ProbeController.php", "trusted"),
    "escaped": ("src/Controller/ProbeController.php", "escaped"),
    "plain": ("src/Controller/ProbeController.php", "plain"),
    "filtered": ("src/Controller/ProbeController.php", "filtered"),
    "template": ("src/Controller/ProbeController.php", "template"),
    "query": ("src/Controller/ProbeController.php", "query"),
    "bound": ("src/Controller/ProbeController.php", "bound"),
    "route-argument": ("src/Controller/ProbeController.php", "routeValue"),
    "form": ("src/Form/ProbeForm.php", "buildForm"),
    "hook": ("phpwarden_taint_probe.module", "phpwarden_taint_probe_preprocess_html"),
    "queue": ("src/Plugin/QueueWorker/ProbeQueue.php", "processItem"),
}


def fingerprint(project):
    paths = [
        project / "composer.json",
        project / "composer.lock",
        project / "web/core/lib/Drupal.php",
    ]
    return {str(p.relative_to(project)): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}


def command(args, project, container, script, *arguments):
    return [
        "docker",
        "run",
        "--pull",
        "never",
        "--rm",
        "--name",
        container,
        "--network",
        "none",
        "--read-only",
        "--user",
        f"{os.getuid()}:{os.getgid()}",
        "--cap-drop",
        "ALL",
        "--security-opt",
        "no-new-privileges",
        "--pids-limit",
        "256",
        "--memory",
        "2g",
        "--tmpfs",
        "/tmp:rw,nosuid,size=1024m,mode=1777",
        "--mount",
        f"type=bind,src={project},dst=/project",
        "--mount",
        f"type=bind,src={args.bundle},dst=/tool-vendor,readonly",
        "--workdir",
        "/project/web",
        "--entrypoint",
        args.php,
        args.image,
        script,
        *arguments,
    ]


def write_config(project, major, mode):
    autoloader = (
        "/tool-vendor/mortenson/psalm-plugin-drupal/scripts/autoload.php"
        if mode == "upstream-bootstrap"
        else "phpwarden-spike-autoload.php"
    )
    metadata = (
        "<containerXml>phpwarden-spike-container.xml</containerXml>"
        if mode in ("static-metadata", "explicit-entrypoints")
        else ""
    )
    (project / "web/psalm.xml").write_text(
        '<psalm errorLevel="6" resolveFromConfigFile="true" cacheDirectory="/tmp/psalm-cache" '
        f'autoloader="{autoloader}"><fileExtensions><extension name="php"/>'
        '<extension name="module"/><extension name="theme"/><extension name="inc"/>'
        '</fileExtensions><projectFiles><directory name="modules/custom/phpwarden_taint_probe"/>'
        '<file name="psalm_drupal_entrypoint.module"/></projectFiles><plugins>'
        f'<pluginClass class="Psalm\\SymfonyPsalmPlugin\\Plugin"><symfonyMajorVersion>{major}'
        f"</symfonyMajorVersion>{metadata}</pluginClass>"
        f'<pluginClass class="mortenson\\PsalmPluginDrupal\\Plugin">{metadata}'
        f'<extensions><module name="{MODULE}"/></extensions></pluginClass></plugins></psalm>\n'
    )


def run(args, project, name, script, *arguments):
    print(f"Running {name}", flush=True)
    snapshot = args.output / "inputs" / name
    snapshot.mkdir(parents=True, exist_ok=True)
    for relative in (
        "web/psalm.xml",
        "web/psalm_drupal_entrypoint.module",
        f"web/modules/custom/{MODULE}/src/InputHelper.php",
    ):
        source = project / relative
        if source.is_file():
            shutil.copyfile(source, snapshot / source.name)
    container = "phpwarden-psalm-spike-" + uuid.uuid4().hex[:12]
    try:
        result = subprocess.run(
            command(args, project, container, script, *arguments),
            capture_output=True,
            text=True,
            timeout=240,
        )
    finally:
        subprocess.run(["docker", "rm", "--force", container], capture_output=True, timeout=15)
    (args.output / f"{name}.stdout").write_text(result.stdout)
    (args.output / f"{name}.stderr").write_text(result.stderr)
    try:
        issues = json.loads(result.stdout)
    except json.JSONDecodeError:
        issues = None
    outcome = {"exit": result.returncode, "json": isinstance(issues, list)}
    if isinstance(issues, list):
        outcome["types"] = sorted({issue["type"] for issue in issues})
        outcome["findings"] = len(issues)
        outcome["non_taint_types"] = sorted(
            {issue["type"] for issue in issues if not issue["type"].startswith("Tainted")}
        )
    outcome["taint_only_json"] = (
        result.returncode in (0, 2) and isinstance(issues, list) and not outcome["non_taint_types"]
    )
    print(name, json.dumps(outcome), flush=True)
    return outcome


def controls(project, issues):
    result = {}
    for label, (name, method) in CONTROLS.items():
        relative = f"modules/custom/{MODULE}/{name}"
        lines = (project / "web" / relative).read_text().splitlines()
        start = next(i for i, line in enumerate(lines, 1) if f"function {method}(" in line)
        end = next(
            (
                i
                for i, line in enumerate(lines, 1)
                if i > start and re.search(r"\bfunction \w+\(", line)
            ),
            len(lines) + 1,
        )
        types = set()
        for issue in issues:
            locations = [issue, *issue.get("taint_trace", [])]
            if any(
                location.get("file_name") == relative
                and start <= location.get("line_from", 0) < end
                for location in locations
            ):
                types.add(issue["type"])
        result[label] = sorted(types)
    return result


def analyze(args, project, name, *, taint=True):
    arguments = [
        "--root=/project",
        "--config=/project/web/psalm.xml",
        "--no-cache",
        "--no-diff",
        "--no-progress",
        "--threads=1",
        "--scan-threads=1",
        "--memory-limit=1536M",
        "--output-format=json",
    ]
    if taint:
        arguments.append("--taint-analysis")
    result = run(args, project, name, "/tool-vendor/bin/psalm", *arguments)
    if result["json"] and taint:
        result["controls"] = controls(
            project, json.loads((args.output / f"{name}.stdout").read_text())
        )
    return result


def prerequisites(summary):
    return bool(
        summary.get("probe_preserved")
        and summary.get("original_preserved")
        and summary.get("coverage_controls_passed")
        and summary.get("failure_controls_passed")
        and summary.get("readiness_clean")
    )


def main():
    parser = argparse.ArgumentParser(
        description="Evaluate upstream Psalm plugin on private Drupal copies"
    )
    parser.add_argument(
        "--project", type=Path, required=True, help="Installed locked Drupal fixture"
    )
    parser.add_argument("--bundle", type=Path, required=True, help="Installed upstream tool vendor")
    parser.add_argument("--image", required=True)
    parser.add_argument("--php", default="php")
    parser.add_argument("--output", type=Path, required=True, help="New artifact directory")
    args = parser.parse_args()
    args.project = args.project.resolve(strict=True)
    args.bundle = args.bundle.resolve(strict=True)
    args.output = args.output.resolve()
    packages = json.loads((args.project / "composer.lock").read_text())["packages"]
    core = next(p["version"] for p in packages if p["name"] == "drupal/core")
    symfony = next(p["version"] for p in packages if p["name"] == "symfony/http-kernel")
    if core.split(".")[0] not in ("10", "11"):
        parser.error("Only Drupal 10/11 fixtures are supported")
    installed = json.loads((args.bundle / "composer/installed.json").read_text())["packages"]
    plugin = next(p for p in installed if p["name"] == "mortenson/psalm-plugin-drupal")
    psalm = next(p for p in installed if p["name"] == "vimeo/psalm")
    if plugin["source"]["reference"] != PLUGIN_REF or psalm["version"] != "6.20.3":
        parser.error("Tool bundle does not match the pinned Psalm/plugin")
    before = fingerprint(args.project)
    args.output.mkdir(parents=True, exist_ok=False)
    summary = {"drupal": core, "symfony": symfony, "plugin": PLUGIN_REF, "cases": {}}
    project = args.output / "project"
    project.mkdir()
    try:
        print(
            f"Copying Drupal {core} dependencies and core without site settings or database",
            flush=True,
        )
        for name in ("composer.json", "composer.lock", "web/autoload.php"):
            target = project / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(args.project / name, target)
        for name in ("vendor", "web/core"):
            shutil.copytree(args.project / name, project / name, symlinks=True)
        (project / "web/sites/default").mkdir(parents=True)
        shutil.copytree(
            ROOT / "tools/psalm-spike/drupal-fixture", project / f"web/modules/custom/{MODULE}"
        )
        (project / "web/phpwarden-spike-autoload.php").write_text(
            "<?php\n$loader = require 'autoload.php';\n"
            "$loader->addPsr4('Drupal\\\\phpwarden_taint_probe\\\\', "
            "__DIR__ . '/modules/custom/phpwarden_taint_probe/src');\n"
            "return $loader;\n"
        )
        (project / "web/phpwarden-spike-container.xml").write_text(
            '<container xmlns="http://symfony.com/schema/dic/services"><services>'
            '<service id="renderer" class="Drupal\\Core\\Render\\Renderer"/>'
            "</services></container>\n"
        )
        identity = run(
            args,
            project,
            "runtime-identity",
            "-r",
            "require 'phpwarden-spike-autoload.php'; echo json_encode(["
            "'core'=>Drupal::VERSION,'php'=>PHP_VERSION,'form_state_implements'=>"
            "is_a(Drupal\\Core\\Form\\FormState::class,"
            "Drupal\\Core\\Form\\FormStateInterface::class,true)]);",
        )
        if identity["exit"] != 0:
            raise RuntimeError("Fixture runtime identity check failed")
        summary["runtime"] = json.loads((args.output / "runtime-identity.stdout").read_text())
        if summary["runtime"]["core"] != core or not summary["runtime"]["form_state_implements"]:
            raise RuntimeError("Installed fixture identity does not match its lock")
        entrypoint = run(
            args,
            project,
            "entrypoint",
            "/tool-vendor/mortenson/psalm-plugin-drupal/scripts/generate_entrypoint.php",
            f"modules/custom/{MODULE}",
        )
        if entrypoint["exit"] != 0:
            raise RuntimeError("Upstream entrypoint generation failed")
        sources = {
            p: hashlib.sha256(p.read_bytes()).hexdigest()
            for p in (project / f"web/modules/custom/{MODULE}").rglob("*")
            if p.is_file()
        }
        entry_path = project / "web/psalm_drupal_entrypoint.module"
        original_entry = entry_path.read_text()
        for mode in (
            "upstream-bootstrap",
            "module-autoload",
            "static-metadata",
            "explicit-entrypoints",
        ):
            if mode == "explicit-entrypoints":
                entry_path.write_text(
                    original_entry.replace(
                        "->routeValue()",
                        "->routeValue(\\Drupal\\phpwarden_taint_probe\\InputHelper::name())",
                    )
                    + "\n$queue = new \\Drupal\\phpwarden_taint_probe\\Plugin\\QueueWorker\\"
                    "ProbeQueue"
                    "([], 'probe', []);\n" + "$queue->processItem(['name' => "
                    "\\Drupal\\phpwarden_taint_probe\\InputHelper::name()]);\n"
                )
            write_config(project, symfony.lstrip("v").split(".")[0], mode)
            summary["cases"][mode] = analyze(args, project, mode)
            if mode in ("static-metadata", "explicit-entrypoints"):
                summary["cases"][mode + "-readiness"] = analyze(
                    args, project, mode + "-readiness", taint=False
                )
        autoload = project / "vendor/autoload.php"
        saved = autoload.read_bytes()
        try:
            autoload.unlink()
            summary["cases"]["missing-autoload"] = analyze(args, project, "missing-autoload")
        finally:
            autoload.write_bytes(saved)
        helper = project / f"web/modules/custom/{MODULE}/src/InputHelper.php"
        saved = helper.read_bytes()
        try:
            helper.write_bytes(saved + b"\nfunction broken( {\n")
            summary["cases"]["parse-error"] = analyze(args, project, "parse-error")
            summary["cases"]["parse-error-lint"] = run(
                args,
                project,
                "parse-error-lint",
                "-l",
                f"/project/web/modules/custom/{MODULE}/src/InputHelper.php",
            )
            summary["cases"]["parse-error-readiness"] = analyze(
                args, project, "parse-error-readiness", taint=False
            )
        finally:
            helper.write_bytes(saved)
        summary["probe_preserved"] = all(
            hashlib.sha256(p.read_bytes()).hexdigest() == digest for p, digest in sources.items()
        )
        summary["site_settings_present"] = (project / "web/sites/default/settings.php").exists()
        expected = {
            "module-autoload": {"trusted", "query", "form", "hook"},
            "static-metadata": {"trusted", "query", "form", "hook", "template"},
            "explicit-entrypoints": {
                "trusted",
                "query",
                "form",
                "hook",
                "template",
                "route-argument",
                "queue",
            },
        }
        summary["coverage_controls_passed"] = all(
            summary["cases"][mode]["taint_only_json"]
            and {name for name, types in summary["cases"][mode]["controls"].items() if types}
            == labels
            for mode, labels in expected.items()
        )
        summary["failure_controls_passed"] = (
            not summary["cases"]["missing-autoload"]["taint_only_json"]
            and summary["cases"]["parse-error-lint"]["exit"] != 0
            and "ParseError" in summary["cases"]["parse-error-readiness"].get("non_taint_types", [])
        )
        summary["upstream_bootstrap_supported"] = summary["cases"]["upstream-bootstrap"][
            "taint_only_json"
        ]
        readiness = summary["cases"]["explicit-entrypoints-readiness"]
        summary["readiness_clean"] = readiness["json"] and readiness["exit"] == 0
        summary["integration_ready"] = False
    finally:
        summary["original_preserved"] = before == fingerprint(args.project)
        summary["candidate_prerequisites_met"] = prerequisites(summary)
        (args.output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    return 0 if summary["candidate_prerequisites_met"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
