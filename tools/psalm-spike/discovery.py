import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import uuid
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from drupal import CONTROLS, controls  # noqa: E402

from tools.sandbox.workspace import finish  # noqa: E402


def digest(project):
    return {
        str(path.relative_to(project)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in project.rglob("*")
        if path.is_file() and "vendor" not in path.parts and "core" not in path.parts
    }


class Discovery:
    def __init__(self, args):
        self.args = args
        self.root = args.output.resolve()
        self.root.mkdir(parents=True, exist_ok=False)
        self.project = self.root / "project"
        self.summary = {"passed": False, "cases": {}}
        self.composer_root = "."
        self.name = "catalog_review"
        self.module = self.project / "web/modules/custom" / self.name
        self.keep_workspace = args.keep_workspace

    def run(self, arguments, label, expected=(0,), cwd=None):
        result = subprocess.run(
            list(map(str, arguments)), cwd=cwd, capture_output=True, text=True, timeout=240
        )
        (self.root / (label + ".stdout")).write_text(result.stdout)
        (self.root / (label + ".stderr")).write_text(result.stderr)
        if result.returncode not in expected:
            raise RuntimeError(f"{label}: exit {result.returncode}: {result.stderr[-1500:]}")
        return result

    def container(self, script, arguments, output, label, expected=(0,), writable=False):
        name = "phpwarden-psalm-discovery-" + uuid.uuid4().hex[:10]
        command = [
            "docker",
            "run",
            "--pull",
            "never",
            "--rm",
            "--name",
            name,
            "--network",
            "none",
            "--read-only",
            "--user",
            f"{os.getuid()}:{os.getgid()}",
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges",
            "--memory",
            "2g",
            "--pids-limit",
            "256",
            "--tmpfs",
            "/tmp:rw,nosuid,size=1024m,mode=1777",
        ]
        for source, target, readonly in (
            (self.project, "/project", True),
            (self.args.bundle, "/tool-vendor", True),
            (ROOT / "tools/psalm/drupal", "/helper", True),
            (ROOT / "tools/psalm-spike/models", "/models", True),
            (ROOT / "tools/psalm-spike/selection.php", "/selection.php", True),
            (output, "/out", not writable),
            (self.root / "request.json", "/request.json", True),
        ):
            command += [
                "--mount",
                f"type=bind,src={source},dst={target}" + (",readonly" if readonly else ""),
            ]
        command += [
            "--workdir",
            "/out",
            "--entrypoint",
            self.args.php,
            self.args.image,
            script,
            *arguments,
        ]
        try:
            return self.run(command, label, expected)
        finally:
            subprocess.run(["docker", "rm", "--force", name], capture_output=True, timeout=20)

    def prepare(self, label, *, expected=0, code=None, native=False):
        effective = json.loads(
            self.run(
                [self.args.binary, "config", "show", "--project", self.project, "--format=json"],
                label + "-facts",
            ).stdout
        )["effective"]
        config, facts = effective["config"], effective["project"]
        request = {
            "root": "/project",
            "composer_root": config["composer_root"],
            "web_root": facts["web_root"],
            "source_roots": facts["source_roots"],
            "include": config["scope"]["include"],
            "exclude": config["scope"]["exclude"],
        }
        (self.root / "request.json").write_text(json.dumps(request))
        output = self.root / label
        output.mkdir()
        before = digest(self.project)
        result = self.container(
            "/helper/prepare.php",
            ["/request.json", "/out"],
            output,
            label,
            (expected,),
            writable=True,
        )
        manifest = json.loads(result.stdout) if result.stdout.strip() else None
        assert before == digest(self.project), "Preparation changed project files"
        if expected == 0:
            assert manifest["preparation_complete"] and not manifest["analysis_complete"]
            assert all(route["modeled"] for route in manifest["routes"])
            assert manifest["services"]["renderer"] == "Drupal\\Core\\Render\\Renderer"
            assert (
                manifest["services"][self.name + ".alias"]
                == f"Drupal\\{self.name}\\Controller\\ProbeController"
            )
            assert "$_GET['name']" in (output / "entrypoints.php").read_text()
            assert "new " not in (output / "entrypoints.php").read_text()
        if code:
            assert manifest and any(item["code"] == code for item in manifest["diagnostics"]), (
                manifest
            )
        record = {"exit": result.returncode, "manifest": manifest is not None}
        if manifest:
            record.update(routes=len(manifest["routes"]), diagnostics=manifest["diagnostics"])
        if native:
            arguments = [
                "--root=/project/" + self.composer_root,
                "--config=/out/psalm.xml",
                "--no-cache",
                "--no-diff",
                "--no-progress",
                "--threads=1",
                "--scan-threads=1",
                "--memory-limit=1536M",
                "--output-format=json",
            ]
            ready = self.container("/models/psalm.php", arguments, output, label + "-readiness")
            assert json.loads(ready.stdout) == []
            taint = self.container(
                "/models/psalm.php",
                [*arguments, "--taint-analysis"],
                output,
                label + "-taint",
                (2,),
            )
            issues = json.loads(taint.stdout)
            assert all(item["type"].startswith("Tainted") for item in issues)
            assert len(issues) == 7, issues
            measured = controls(
                self.project / self.composer_root,
                issues,
                module=self.name,
                labels=set(CONTROLS) - {"queue", "queue-bound"},
                container_root="/project"
                + ("/" + self.composer_root if self.composer_root != "." else ""),
            )
            assert {name for name, types in measured.items() if types} == {
                "trusted",
                "query",
                "form",
                "hook",
                "template",
                "route-argument",
            }, measured
            record.update(readiness_clean=True, taint_findings=len(issues), controls=measured)
        self.summary["cases"][label] = record
        print(label, json.dumps(record), flush=True)
        return manifest

    def seed(self):
        for name in ("composer.json", "composer.lock", "web/autoload.php"):
            target = self.project / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(self.args.project / name, target)
        for name in ("vendor", "web/core"):
            shutil.copytree(self.args.project / name, self.project / name, symlinks=True)
        shutil.copytree(ROOT / "tools/psalm-spike/drupal-fixture", self.module)
        for file in sorted(self.module.rglob("*"), reverse=True):
            if file.is_file():
                file.write_text(file.read_text().replace("phpwarden_taint_probe", self.name))
            if "phpwarden_taint_probe" in file.name:
                file.rename(file.with_name(file.name.replace("phpwarden_taint_probe", self.name)))
        shutil.rmtree(self.module / "src/Plugin")
        controller = self.module / "src/Controller/ProbeController.php"
        controller.write_text(
            controller.read_text()
            .replace(
                "final class ProbeController {",
                "final class ProbeController {\n"
                "  public function __construct(\\stdClass $dependency) "
                "{ $dependency->ready = true; }",
            )
            .replace(
                "routeValue(string $name)",
                "routeValue(string $name, string $context, string $suffix = 'default')",
            )
            .replace("echo $name;", "echo $name;\n    echo Html::escape($context . $suffix);")
        )
        (self.module / (self.name + ".services.yml")).write_text(
            f"parameters:\n  controller.class: Drupal\\{self.name}\\Controller\\ProbeController\n"
            f"services:\n  {self.name}.controller:\n    class: '%controller.class%'\n"
            f"  {self.name}.alias:\n    alias: {self.name}.controller\n"
        )
        routing = self.module / (self.name + ".routing.yml")
        routing.write_text(
            routing.read_text().replace(
                f"'\\Drupal\\{self.name}\\Controller\\ProbeController::routeValue'",
                "'" + self.name + ".alias::routeValue'\n    context: safe",
            )
        )
        self.config()

    def config(self, suffix=""):
        (self.project / ".phpwarden.yaml").write_text(
            'version: 1\nprofile: drupal\nphp: {runtime: "8.3", target: {min: "8.3", max: "8.3"}}\n'
            + "composer_root: "
            + self.composer_root
            + "\n"
            + suffix
        )

    def negative_cases(self):
        controller = self.module / "src/Controller/ProbeController.php"
        original = controller.read_text()
        cases = {
            "dynamic-service": (
                original.replace(
                    "public function trusted(): array {",
                    "public function trusted(): array {\n"
                    "    \\Drupal::service(InputHelper::name());",
                ),
                "dynamic-service",
            ),
            "missing-service": (
                original.replace(
                    "public function trusted(): array {",
                    "public function trusted(): array {\n    \\Drupal::service('unknown.service');",
                ),
                "service-unresolved",
            ),
            "parser-error": (original + "\nfunction broken( {\n", None),
            "entity-conversion": (
                original.replace("routeValue(string $name,", "routeValue(\\stdClass $name,"),
                "route-conversion",
            ),
        }
        try:
            for label, (text, code) in cases.items():
                controller.write_text(text)
                self.prepare(label, expected=3, code=code)
        finally:
            controller.write_text(original)
        service = self.module / (self.name + ".services.yml")
        original = service.read_text()
        try:
            service.write_text(original + "  broken: [\n")
            self.prepare("yaml-error", expected=3)
            service.write_text(
                original.replace(
                    "alias: " + self.name + ".controller", "alias: " + self.name + ".alias"
                )
            )
            self.prepare("alias-cycle", expected=3, code="service-unresolved")
        finally:
            service.write_text(original)
        queue = self.module / "src/Plugin/QueueWorker/ProbeQueue.php"
        queue.parent.mkdir(parents=True)
        try:
            queue.write_text(
                (ROOT / "tools/psalm-spike/drupal-fixture/src/Plugin/QueueWorker/ProbeQueue.php")
                .read_text()
                .replace("phpwarden_taint_probe", self.name)
            )
            self.prepare("queue-unmodeled", expected=3, code="queue-entrypoint")
        finally:
            queue.unlink()
        external = self.root / "external.php"
        external.write_text("<?php original sentinel")
        link = self.module / "src/External.php"
        try:
            link.symlink_to(external)
            self.prepare("source-symlink", expected=3, code="symlink")
            assert external.read_text() == "<?php original sentinel"
        finally:
            link.unlink()
            external.unlink()

    def monorepo(self):
        app = self.project / "apps/storefront"
        app.mkdir(parents=True)
        for name in ("composer.json", "composer.lock", "vendor", "web"):
            (self.project / name).rename(app / name)
        self.composer_root = "apps/storefront"
        self.module = app / "web/modules/custom" / self.name
        package = self.project / "packages/outside_module"
        package.mkdir(parents=True)
        (package / "outside_module.info.yml").write_text(
            "name: Outside module\ntype: module\ncore_version_requirement: ^10 || ^11\n"
        )
        (package / "composer.json").write_text(
            json.dumps({"name": "fixture/outside-module", "type": "drupal-custom-module"})
        )
        (package / "src").mkdir()
        (package / "src/OutsideController.php").write_text(
            "<?php namespace Drupal\\outside_module; final class OutsideController {"
            " public function content(): array {return ['#plain_text' => "
            "(string) \\Drupal::request()->query->get('name')];}}\n"
        )
        (package / "outside_module.services.yml").write_text(
            "services:\n  outside.controller:\n"
            "    class: Drupal\\outside_module\\OutsideController\n"
        )
        (package / "outside_module.routing.yml").write_text(
            "outside.content:\n  path: /outside\n  defaults:\n"
            "    _controller: 'outside.controller:content'\n  requirements:\n    _access: 'TRUE'\n"
        )
        composer = app / "composer.json"
        body = json.loads(composer.read_text())
        body["repositories"].append({"type": "path", "url": "../../packages/*"})
        composer.write_text(json.dumps(body))
        self.config()
        manifest = self.prepare("monorepo", native=True)
        assert any(item["path"] == "packages/outside_module" for item in manifest["extensions"])
        self.config('scope: {exclude: ["packages/outside_module/**"]}\n')
        manifest = self.prepare("excluded-module")
        assert all(item["name"] != "outside_module" for item in manifest["extensions"])
        self.config('scope: {include: ["apps/storefront/web/modules/custom/catalog_review"]}\n')
        manifest = self.prepare("included-module")
        assert len(manifest["extensions"]) == 1 and len(manifest["routes"]) == 9

    def execute(self):
        try:
            self.seed()
            output = self.root / "selection"
            output.mkdir()
            (self.root / "request.json").write_text("{}")
            result = self.container("/selection.php", [], output, "selection")
            self.summary["cases"]["selection"] = json.loads(result.stdout)
            manifest = self.prepare("automatic", native=True)
            assert len(manifest["routes"]) == 9
            self.negative_cases()
            self.monorepo()
            self.summary["passed"] = True
        finally:
            try:
                finish(self, ("project",), check_mounts=True)
            finally:
                (self.root / "summary.json").write_text(json.dumps(self.summary, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description="Verify database-free Drupal Psalm preparation")
    for name in ("project", "bundle", "binary", "output"):
        parser.add_argument("--" + name, required=True, type=Path)
    parser.add_argument("--image", required=True)
    parser.add_argument("--php", default="php")
    parser.add_argument("--keep-workspace", action="store_true")
    args = parser.parse_args()
    for name in ("project", "bundle", "binary"):
        setattr(args, name, getattr(args, name).resolve(strict=True))
    Discovery(args).execute()


if __name__ == "__main__":
    main()
