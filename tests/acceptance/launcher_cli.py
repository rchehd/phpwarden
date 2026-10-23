import json
import shutil
import subprocess
import tempfile
import unittest
import uuid
from pathlib import Path

from tests.acceptance.docker_worker import snapshot
from tests.support.scenarios import materialize
from tests.support.syntax import check_syntax_fixtures, check_syntax_settings
from tests.support.yaml import FIXTURES as YAML_FIXTURES
from tests.support.yaml import check_yaml_fixtures, check_yaml_settings

ROOT = Path(__file__).resolve().parents[2]
FIXTURES = ROOT / "tests" / "fixtures" / "scenarios"


class LauncherCLITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="phpwarden-launcher-")
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.binary = Path(cls.temporary.name) / "phpwarden"
        image = "phpwarden:static-php8.4-dev"
        container = "phpwarden-cli-test-" + uuid.uuid4().hex
        try:
            subprocess.run(
                ["docker", "create", "--name", container, image],
                capture_output=True,
                check=True,
                timeout=20,
            )
            subprocess.run(
                ["docker", "cp", f"{container}:/usr/local/bin/phpwarden", str(cls.binary)],
                capture_output=True,
                check=True,
                timeout=20,
            )
        finally:
            subprocess.run(["docker", "rm", "--force", container], capture_output=True, timeout=15)
        cls.binary.chmod(0o755)

    def configure(self, root, base, *, include=None, **debug):
        config = {
            "version": 1,
            "profile": "drupal",
            "php": {"runtime": "8.4", "target": {"min": "8.4", "max": "8.4"}},
            "scope": {"include": include or ["web/modules/custom"], "base": base},
            "tools": {"debug": debug},
        }
        (root / ".phpwarden.yaml").write_text(json.dumps(config))

    def run_cli(self, root, args, env):
        with tempfile.TemporaryDirectory(prefix="phpwarden-client-") as temporary:
            for tool in ("git", "docker"):
                (Path(temporary) / tool).symlink_to(shutil.which(tool))
            env = env | {"PATH": temporary}
            self.assertIsNone(shutil.which("php", path=temporary))
            self.assertIsNone(shutil.which("go", path=temporary))
            completed = subprocess.run(
                [str(self.binary), *args, "--format", "json"],
                cwd=root,
                env=env,
                capture_output=True,
                text=True,
                timeout=60,
            )
        self.assertEqual(completed.stderr, "", completed.stderr)
        result = json.loads(completed.stdout)
        self.assertEqual(completed.returncode, result["exit_code"], result)
        return result

    def test_syntax_findings_include_errors_outside_diff(self):
        def configure(scenario):
            self.configure(scenario.root, scenario.base)

        def run(scenario, arguments):
            before = snapshot(scenario.root)
            result = self.run_cli(scenario.root, ["check", *arguments], scenario.env)
            self.assertEqual(snapshot(scenario.root), before)
            self.assertEqual(result["environment"]["protocol"], 2)
            return result

        check_syntax_fixtures(self, configure, run)

    def test_syntax_settings_and_explicit_selection(self):
        check_syntax_settings(
            self,
            lambda scenario: self.configure(scenario.root, scenario.base),
            lambda scenario, arguments: self.run_cli(
                scenario.root, ["check", *arguments], scenario.env
            ),
        )

    def test_yaml_fixtures(self):
        def run(scenario, arguments):
            before = snapshot(scenario.root)
            result = self.run_cli(scenario.root, ["check", *arguments], scenario.env)
            self.assertEqual(snapshot(scenario.root), before)
            return result

        check_yaml_fixtures(
            self, lambda scenario: self.configure(scenario.root, scenario.base), run
        )

    def test_yaml_settings(self):
        check_yaml_settings(
            self,
            lambda scenario: self.configure(scenario.root, scenario.base),
            lambda scenario, arguments: self.run_cli(
                scenario.root, ["check", *arguments], scenario.env
            ),
        )

    def test_yaml_global_error_is_visible_in_text(self):
        with materialize(YAML_FIXTURES / "yaml-invalid-utf8") as scenario:
            self.configure(scenario.root, scenario.base)
            completed = subprocess.run(
                [str(self.binary), "check", "--only", "yaml"],
                cwd=scenario.root,
                env=scenario.env,
                capture_output=True,
                text=True,
                timeout=60,
            )
            self.assertEqual(completed.returncode, 1, completed.stdout)
            self.assertEqual(completed.stderr, "")
            self.assertIn("YAML file", completed.stdout)
            self.assertIn("encoding.yaml", completed.stdout)
            self.assertIn("UTF-8", completed.stdout)

    def test_public_check_matches_all_fixture_modes_without_host_php_or_go(self):
        for directory in sorted(FIXTURES.iterdir()):
            for mode in ("diff", "full", "whole-files", "paths"):
                with self.subTest(scenario=directory.name, mode=mode):
                    with materialize(directory) as scenario:
                        self.configure(scenario.root, scenario.base)
                        before = snapshot(scenario.root)
                        options = {
                            "diff": [],
                            "full": ["--mode", "full"],
                            "whole-files": ["--whole-files"],
                            "paths": scenario.spec["changed_files"],
                        }[mode]
                        result = self.run_cli(scenario.root, ["check", *options], scenario.env)
                        expected_mode = "diff" if mode == "diff" else "full"
                        verdict = scenario.spec["verdicts"][expected_mode]
                        self.assertEqual(result["verdict"], verdict, result)
                        self.assertEqual(result["exit_code"], 1 if verdict == "fail" else 0)
                        self.assertTrue(result["complete"])
                        self.assertTrue(result["environment"]["image"].startswith("sha256:"))
                        self.assertEqual(result["environment"]["toolset"], "static-dev-v2-php8.4")
                        expected = sorted(
                            (row["checker"], row["path"], row["line"])
                            for row in scenario.spec["findings"]
                            if expected_mode in row["modes"]
                        )
                        actual = sorted(
                            (row["checker"], row["path"], row["line"]) for row in result["findings"]
                        )
                        self.assertEqual(actual, expected)
                        self.assertEqual(snapshot(scenario.root), before)

    def test_linked_worktree_with_external_git_metadata(self):
        with materialize(FIXTURES / "clean-debug") as scenario:
            with tempfile.TemporaryDirectory(prefix="phpwarden-worktree-") as temporary:
                root = Path(temporary) / "linked tree, with spaces"
                subprocess.run(
                    ["git", "worktree", "add", "--detach", str(root), scenario.base],
                    cwd=scenario.root,
                    env=scenario.env,
                    capture_output=True,
                    check=True,
                    timeout=15,
                )
                try:
                    self.configure(root, scenario.base)
                    source = root / "web/modules/custom/demo/demo.module"
                    source.write_text(source.read_text() + "\nvar_dump(1);\n")
                    before = snapshot(root)
                    result = self.run_cli(root, ["check"], scenario.env)
                    self.assertEqual(result["exit_code"], 1, result)
                    self.assertEqual(len(result["findings"]), 1)
                    self.assertEqual(snapshot(root), before)
                finally:
                    subprocess.run(
                        ["git", "worktree", "remove", "--force", str(root)],
                        cwd=scenario.root,
                        env=scenario.env,
                        capture_output=True,
                        check=True,
                        timeout=15,
                    )

    def test_monorepo_and_explicit_paths_from_subdirectory(self):
        with materialize(FIXTURES / "clean-debug") as scenario:
            project = scenario.root / "apps" / "site with spaces"
            source = project / "src"
            source.mkdir(parents=True)
            (source / "demo.module").write_text("<?php\nvar_dump(1);\n")
            self.configure(project, scenario.base, include=["src"])
            result = self.run_cli(source, ["check", "demo.module"], scenario.env)
            self.assertEqual(result["exit_code"], 1, result)
            self.assertEqual(result["findings"][0]["path"], "src/demo.module")

    def test_resolve_is_offline_and_settings_are_shared(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario.root, scenario.base, enabled=False)
            result = self.run_cli(scenario.root, ["resolve"], scenario.env)
            self.assertTrue(result["valid"])
            self.assertTrue(result["selection"]["development"])
            self.assertEqual(result["selection"]["image"], "phpwarden:static-php8.4-dev")
            self.assertEqual(result["selection"]["release"], "development")
            self.assertEqual(result["selection"]["entry"], "drupal-static-dev-php8.4")
            self.assertEqual(result["selection"]["preset"]["id"], "drupal-static")
            self.assertEqual(result["selection"]["tools"]["php"], "8.4.26")
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 0)
            self.assertEqual(result["checks"][0]["reason"], "disabled")
            result = self.run_cli(scenario.root, ["check", "--only", "debug"], scenario.env)
            self.assertEqual(result["exit_code"], 1, result)
            result = self.run_cli(scenario.root, ["check", "--only", "phpstna"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            filename = scenario.root / ".phpwarden.yaml"
            config = json.loads(filename.read_text())
            config["php"] = {"runtime": "8.3", "target": {"min": "8.3", "max": "8.3"}}
            filename.write_text(json.dumps(config))
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            self.assertEqual(result["diagnostics"][0]["code"], "compatibility_invalid")

    def test_drupal_discovery_and_profile_exclusions(self):
        for web_root in (".", "web", "docroot", "public [site]"):
            with self.subTest(web_root=web_root):
                with materialize(FIXTURES / "clean-debug") as scenario:
                    self.configure(scenario.root, scenario.base)
                    filename = scenario.root / ".phpwarden.yaml"
                    config = json.loads(filename.read_text())
                    del config["scope"]["include"]
                    filename.write_text(json.dumps(config))
                    composer = {
                        "extra": {"drupal-scaffold": {"locations": {"web-root": web_root}}},
                        "scripts": {"post-install-cmd": "touch MUST_NOT_EXIST"},
                    }
                    lock = {
                        "packages": [{"name": "drupal/core", "version": "11.2.5"}],
                        "packages-dev": [],
                    }
                    (scenario.root / "composer.json").write_text(json.dumps(composer))
                    (scenario.root / "composer.lock").write_text(json.dumps(lock))
                    custom = [
                        "modules/custom/example/example.module",
                        "themes/custom/site/site.theme",
                        "profiles/custom/project/project.profile",
                        "sites/client.example/modules/custom/client/client.module",
                    ]
                    excluded = [
                        "core/lib/Drupal.php",
                        "modules/contrib/token/token.module",
                        "themes/contrib/base/base.theme",
                        "sites/client.example/modules/contrib/external/external.module",
                    ]
                    for relative in custom + excluded:
                        source = scenario.root / web_root / relative
                        source.parent.mkdir(parents=True, exist_ok=True)
                        source.write_text("<?php\nvar_dump(1);\n")
                    effective = self.run_cli(scenario.root, ["config", "show"], scenario.env)[
                        "effective"
                    ]
                    self.assertEqual(effective["config"]["framework"]["version"], "11.2.5")
                    self.assertEqual(effective["project"]["web_root"], web_root)
                    self.assertEqual(effective["pending"], [])
                    self.assertEqual(effective["provenance"]["scope.include"], "profile_discovery")
                    expected = sorted(str(Path(web_root) / relative) for relative in custom)
                    before = snapshot(scenario.root)
                    result = self.run_cli(scenario.root, ["check", "--mode", "full"], scenario.env)
                    self.assertEqual(result["exit_code"], 1, result)
                    self.assertEqual(result["environment"]["framework_version"], "11.2.5")
                    self.assertEqual(sorted(row["path"] for row in result["findings"]), expected)
                    self.assertEqual(snapshot(scenario.root), before)
                    config["scope"]["include"] = ["."]
                    filename.write_text(json.dumps(config))
                    result = self.run_cli(scenario.root, ["check", "--mode", "full"], scenario.env)
                    self.assertEqual(result["exit_code"], 1, result)
                    self.assertEqual(sorted(row["path"] for row in result["findings"]), expected)
                    self.assertFalse((scenario.root / "MUST_NOT_EXIST").exists())

    def test_discovery_errors_do_not_pass_or_redefine_config_validation(self):
        with materialize(FIXTURES / "clean-debug") as scenario:
            self.configure(scenario.root, scenario.base)
            composer = scenario.root / "composer.json"
            composer.write_text("{")
            result = self.run_cli(scenario.root, ["config", "validate"], scenario.env)
            self.assertEqual(result["exit_code"], 0)
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            self.assertEqual(result["diagnostics"][0]["code"], "discovery_invalid")
            composer.write_text("{}")
            lock = {"packages": [{"name": "drupal/core", "version": "11.2.5"}]}
            (scenario.root / "composer.lock").write_text(json.dumps(lock))
            filename = scenario.root / ".phpwarden.yaml"
            config = json.loads(filename.read_text())
            config["framework"] = {"version": "10.4.0"}
            filename.write_text(json.dumps(config))
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            self.assertIn("conflicts", result["diagnostics"][0]["message"])

    def test_composer_php_and_installed_version_compatibility(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario.root, scenario.base)
            composer = {
                "require": {"php": "^8.3"},
                "require-dev": {"php": "^8.4"},
                "config": {"vendor-dir": "dependencies", "platform": {"php": "8.4.0"}},
            }
            lock = {
                "packages": [
                    {"name": "drupal/core", "version": "11.2.5", "require": {"php": ">=8.3"}}
                ],
                "packages-dev": [{"name": "test/tool", "require": {"php": "^8.4"}}],
                "platform": {"php": "^8.3"},
                "platform-dev": [],
            }
            composer_file = scenario.root / "composer.json"
            composer_file.write_text(json.dumps(composer))
            (scenario.root / "composer.lock").write_text(json.dumps(lock))
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 1, result)
            self.assertEqual(result["environment"]["php_version"], "8.4.26")
            shown = self.run_cli(scenario.root, ["config", "show"], scenario.env)["effective"]
            self.assertEqual(shown["project"]["installed"]["status"], "absent")
            installed_file = scenario.root / "dependencies/composer/installed.json"
            installed_file.parent.mkdir(parents=True)
            installed = {"packages": [{"name": "drupal/core", "version": "11.2.5"}], "dev": True}
            installed_file.write_text(json.dumps(installed))
            (scenario.root / "dependencies/autoload.php").write_text(
                "<?php var_dump(100); file_put_contents('MUST_NOT_EXIST', 'executed');\n"
            )
            before = snapshot(scenario.root)
            result = self.run_cli(scenario.root, ["check", "--mode", "full"], scenario.env)
            self.assertEqual(result["exit_code"], 1, result)
            self.assertEqual(len(result["findings"]), 1)
            self.assertEqual(snapshot(scenario.root), before)
            self.assertFalse((scenario.root / "MUST_NOT_EXIST").exists())
            shown = self.run_cli(scenario.root, ["config", "show"], scenario.env)["effective"]
            self.assertEqual(shown["project"]["installed"]["status"], "verified")
            installed["packages"][0]["version"] = "11.2.6"
            installed_file.write_text(json.dumps(installed))
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            self.assertIn("installed", result["diagnostics"][0]["message"])
            installed["packages"][0]["version"] = "11.2.5"
            installed_file.write_text(json.dumps(installed))
            composer["require"]["php"] = ">=8.5"
            composer_file.write_text(json.dumps(composer))
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            self.assertIn("PHP runtime", result["diagnostics"][0]["message"])
            composer["require"]["php"] = ">=8.4.27 <9"
            composer["config"]["platform"]["php"] = "8.4.27"
            composer_file.write_text(json.dumps(composer))
            shown = self.run_cli(scenario.root, ["config", "show"], scenario.env)["effective"]
            self.assertIn("php.runtime.patch", shown["pending"])
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 2)
            self.assertEqual(result["diagnostics"][0]["code"], "compatibility_invalid")
            self.assertIn("8.4.26", result["diagnostics"][0]["message"])
            composer["require"]["php"] = ">=8.4.25 <9"
            composer["config"]["platform"]["php"] = "8.4.25"
            composer_file.write_text(json.dumps(composer))
            result = self.run_cli(scenario.root, ["check"], scenario.env)
            self.assertEqual(result["exit_code"], 1, result)
            self.assertEqual(result["environment"]["compatibility_pending"], ["php.target.patch"])


if __name__ == "__main__":
    unittest.main()
