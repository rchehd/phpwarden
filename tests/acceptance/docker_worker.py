import csv
import hashlib
import io
import json
import os
import shutil
import subprocess
import tempfile
import unittest
import uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from tests.support.composer import (
    check_composer_audit,
    check_composer_validate_fixtures,
    check_composer_validate_settings,
)
from tests.support.drupal_config import (
    check_drupal_config_custom_layout,
    check_drupal_config_fixtures,
    check_drupal_config_settings,
)
from tests.support.phpcs import (
    check_phpcs_fixtures,
    check_phpcs_project_rules,
    check_phpcs_settings,
)
from tests.support.phpstan import (
    check_phpstan_dependency_mismatch,
    check_phpstan_fixtures,
    check_phpstan_project_config,
    check_phpstan_settings,
)
from tests.support.scenarios import materialize
from tests.support.syntax import check_syntax_fixtures
from tests.support.twig_cs_fixer import (
    check_twig_cs_fixer_fixtures,
    check_twig_cs_fixer_project_config,
    check_twig_cs_fixer_settings,
)
from tests.support.whitespace import check_whitespace_fixtures, check_whitespace_settings
from tests.support.yaml import check_yaml_fixtures

ROOT = Path(__file__).resolve().parents[2]
FIXTURES = ROOT / "tests" / "fixtures" / "scenarios"


def snapshot(root):
    return {
        str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in root.rglob("*")
        if path.is_file()
    }


class DockerWorkerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.image = os.environ.get("PHPWARDEN_TEST_IMAGE", "phpwarden:static-php8.4-dev")
        result = subprocess.run(
            ["docker", "image", "inspect", cls.image],
            capture_output=True,
            text=True,
            check=True,
            timeout=15,
        )
        cls.metadata = json.loads(result.stdout)[0]

    def configure(self, scenario, runtime="8.4"):
        config = {
            "version": 1,
            "profile": "drupal",
            "php": {"runtime": runtime, "target": {"min": runtime, "max": runtime}},
            "scope": {"include": ["web/modules/custom"], "base": scenario.base},
            "tools": {
                "composer-audit": {"enabled": False},
                "composer-validate": {"enabled": False},
                "phpcs": {"enabled": False},
                "phpstan": {"enabled": False},
                "twig-cs-fixer": {"enabled": False},
            },
        }
        (scenario.root / ".phpwarden.yaml").write_text(json.dumps(config))

    def run_container(self, root, arguments, *, entrypoint=None, default_user=False, env=None):
        name = "phpwarden-test-" + uuid.uuid4().hex
        mount = io.StringIO()
        csv.writer(mount, lineterminator="").writerow(
            ["type=bind", f"source={root}", "target=/workspace", "readonly"]
        )
        command = [
            "docker",
            "run",
            "--rm",
            "--pull=never",
            "--name",
            name,
            "--network=none",
            "--read-only",
            "--cap-drop=ALL",
            "--security-opt=no-new-privileges:true",
            "--tmpfs",
            "/tmp:rw,nosuid,nodev,noexec,size=256m,mode=1777",
            "--mount",
            mount.getvalue(),
        ]
        if not default_user:
            command.extend(["--user", f"{os.getuid()}:{os.getgid()}"])
        if entrypoint:
            command.extend(["--entrypoint", entrypoint])
        command.extend([self.image, *arguments])
        try:
            return subprocess.run(command, capture_output=True, text=True, timeout=60, env=env)
        finally:
            subprocess.run(
                ["docker", "rm", "--force", name],
                capture_output=True,
                timeout=15,
                env=env,
            )

    def report(self, completed):
        self.assertEqual(completed.stderr, "", completed.stderr)
        result = json.loads(completed.stdout)
        self.assertEqual(result["exit_code"], completed.returncode, result)
        return result

    def test_real_image_matches_all_fixture_modes(self):
        for directory in sorted(FIXTURES.iterdir()):
            for mode in ("diff", "full", "whole-files", "paths"):
                with self.subTest(scenario=directory.name, mode=mode):
                    with materialize(directory) as scenario:
                        self.configure(scenario)
                        before = snapshot(scenario.root)
                        options = {
                            "diff": [],
                            "full": ["--mode", "full"],
                            "whole-files": ["--whole-files"],
                            "paths": scenario.spec["changed_files"],
                        }[mode]
                        result = self.report(
                            self.run_container(
                                scenario.root, ["worker", "check", "--format", "json", *options]
                            )
                        )
                        expected_mode = "diff" if mode == "diff" else "full"
                        verdict = scenario.spec["verdicts"][expected_mode]
                        self.assertEqual(result["verdict"], verdict)
                        self.assertEqual(result["exit_code"], 1 if verdict == "fail" else 0)
                        self.assertTrue(result["complete"])
                        self.assertEqual(result["environment"]["php_runtime"], "8.4")
                        actual = sorted(
                            (row["checker"], row["path"], row["line"]) for row in result["findings"]
                        )
                        expected = sorted(
                            (row["checker"], row["path"], row["line"])
                            for row in scenario.spec["findings"]
                            if expected_mode in row["modes"]
                        )
                        self.assertEqual(actual, expected)
                        self.assertEqual(snapshot(scenario.root), before)

    def test_default_user_and_read_only_project(self):
        self.assertEqual(self.metadata["Config"]["User"], "10001:10001")
        self.assertEqual(
            self.metadata["Config"]["Labels"]["io.phpwarden.checkers"],
            "composer-audit,composer-validate,debug,drupal-config,phpcs,phpstan,syntax,twig-cs-fixer,whitespace,yaml",
        )
        with materialize(FIXTURES / "clean-debug") as scenario:
            self.configure(scenario)
            scenario.root.chmod(0o755)
            completed = self.run_container(
                scenario.root, ["-u"], entrypoint="/usr/bin/id", default_user=True
            )
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertEqual(completed.stdout.strip(), "10001")
            result = self.report(
                self.run_container(
                    scenario.root,
                    ["worker", "check", "--format", "json"],
                    default_user=True,
                )
            )
            self.assertEqual(result["exit_code"], 0)
            completed = self.run_container(
                scenario.root, ["-c", "touch /workspace/unexpected"], entrypoint="/bin/sh"
            )
            self.assertNotEqual(completed.returncode, 0)
            self.assertIn("read-only", completed.stderr.lower())
            self.assertFalse((scenario.root / "unexpected").exists())

    def test_syntax_findings_include_errors_outside_diff(self):
        def run(scenario, arguments):
            before = snapshot(scenario.root)
            result = self.report(
                self.run_container(
                    scenario.root, ["worker", "check", "--format", "json", *arguments]
                )
            )
            self.assertEqual(snapshot(scenario.root), before)
            return result

        check_syntax_fixtures(self, self.configure, run)

    def test_yaml_fixtures(self):
        def run(scenario, arguments):
            before = snapshot(scenario.root)
            result = self.report(
                self.run_container(
                    scenario.root, ["worker", "check", "--format", "json", *arguments]
                )
            )
            self.assertEqual(snapshot(scenario.root), before)
            return result

        check_yaml_fixtures(self, self.configure, run)

    def run_check(self, scenario, arguments):
        before = snapshot(scenario.root)
        result = self.report(
            self.run_container(scenario.root, ["worker", "check", "--format", "json", *arguments])
        )
        self.assertEqual(snapshot(scenario.root), before)
        return result

    def test_whitespace_fixtures(self):
        check_whitespace_fixtures(self, self.configure, self.run_check)

    def test_whitespace_settings(self):
        check_whitespace_settings(self, self.configure, self.run_check)

    def test_drupal_config_fixtures(self):
        check_drupal_config_fixtures(self, self.configure, self.run_check)

    def test_drupal_config_settings(self):
        check_drupal_config_settings(self, self.configure, self.run_check)

    def test_drupal_config_custom_layout(self):
        check_drupal_config_custom_layout(self, self.configure, self.run_check)

    def test_phpcs_fixtures(self):
        check_phpcs_fixtures(self, self.configure, self.run_check)

    def test_phpcs_settings(self):
        check_phpcs_settings(self, self.configure, self.run_check)

    def test_phpcs_project_rules(self):
        check_phpcs_project_rules(self, self.configure, self.run_check)

    def test_phpstan_fixtures(self):
        check_phpstan_fixtures(self, self.configure, self.run_check)

    def test_phpstan_settings(self):
        check_phpstan_settings(self, self.configure, self.run_check)

    def test_phpstan_project_config(self):
        check_phpstan_project_config(self, self.configure, self.run_check)

    def test_phpstan_dependency_mismatch(self):
        check_phpstan_dependency_mismatch(self, self.configure, self.run_check)

    def test_twig_cs_fixer_fixtures(self):
        check_twig_cs_fixer_fixtures(self, self.configure, self.run_check)

    def test_twig_cs_fixer_settings(self):
        check_twig_cs_fixer_settings(self, self.configure, self.run_check)

    def test_twig_cs_fixer_project_config(self):
        check_twig_cs_fixer_project_config(self, self.configure, self.run_check)

    def test_composer_validate_fixtures(self):
        check_composer_validate_fixtures(self, self.configure, self.run_check)

    def test_composer_validate_settings(self):
        check_composer_validate_settings(self, self.configure, self.run_check)

    def test_composer_audit(self):
        check_composer_audit(self, self.configure, self.run_check, network=False)

    def test_no_host_php_or_go_and_no_runtime_build_tools(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario)
            with tempfile.TemporaryDirectory(prefix="phpwarden-client-") as temporary:
                (Path(temporary) / "docker").symlink_to(shutil.which("docker"))
                env = os.environ | {"PATH": temporary}
                self.assertIsNone(shutil.which("php", path=temporary))
                self.assertIsNone(shutil.which("go", path=temporary))
                result = self.report(
                    self.run_container(
                        scenario.root, ["worker", "check", "--format", "json"], env=env
                    )
                )
                self.assertEqual(result["exit_code"], 1)
            completed = self.run_container(
                scenario.root,
                ["-c", "! command -v go && ! command -v python3 && ! command -v composer"],
                entrypoint="/bin/sh",
            )
            self.assertEqual(completed.returncode, 0, completed.stderr)

    def test_runtime_mismatch_is_not_clean(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario, runtime="8.3")
            result = self.report(
                self.run_container(scenario.root, ["worker", "check", "--format", "json"])
            )
            self.assertEqual(result["exit_code"], 3)
            self.assertFalse(result["complete"])
            self.assertIn("runtime_mismatch", [row["code"] for row in result["diagnostics"]])

    def test_two_containers_can_check_the_same_project(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario)
            before = snapshot(scenario.root)
            with ThreadPoolExecutor(max_workers=2) as pool:
                futures = [
                    pool.submit(
                        self.run_container, scenario.root, ["worker", "check", "--format", "json"]
                    )
                    for _ in range(2)
                ]
                reports = [self.report(future.result()) for future in futures]
            self.assertTrue(all(row["exit_code"] == 1 and row["complete"] for row in reports))
            self.assertNotEqual(reports[0]["run_id"], reports[1]["run_id"])
            self.assertEqual(snapshot(scenario.root), before)

    def test_monorepo_project_paths_stay_relative(self):
        with materialize(FIXTURES / "clean-debug") as scenario:
            project = scenario.root / "apps" / "site with spaces"
            project.mkdir(parents=True)
            (project / "example.module").write_text("<?php\nvar_dump(1);\n")
            config = {
                "version": 1,
                "profile": "drupal",
                "php": {"runtime": "8.4", "target": {"min": "8.4", "max": "8.4"}},
                "scope": {"mode": "full", "include": ["."]},
                "tools": {
                    "composer-audit": {"enabled": False},
                    "composer-validate": {"enabled": False},
                    "phpcs": {"enabled": False},
                    "phpstan": {"enabled": False},
                    "twig-cs-fixer": {"enabled": False},
                },
            }
            (project / ".phpwarden.yaml").write_text(json.dumps(config))
            result = self.report(
                self.run_container(
                    scenario.root,
                    [
                        "worker",
                        "check",
                        "--project",
                        "/workspace/apps/site with spaces",
                        "--format",
                        "json",
                    ],
                )
            )
            self.assertEqual(result["exit_code"], 1)
            self.assertEqual(result["findings"][0]["path"], "example.module")


if __name__ == "__main__":
    unittest.main()
