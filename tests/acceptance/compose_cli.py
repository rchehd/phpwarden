import json
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


class ComposeCLI(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        image = os.environ.get("PHPWARDEN_TEST_IMAGE", "phpwarden:static-php8.3-dev")
        result = subprocess.run(
            ["docker", "image", "inspect", image, "--format", "{{.Id}}"],
            capture_output=True,
            text=True,
            check=True,
        )
        cls.image = result.stdout.strip()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="phpwarden-compose-test-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.environment = os.environ | {"XDG_CACHE_HOME": str(self.root / "cache")}
        self.run_command(["git", "init", "-q"])
        self.run_command(["git", "config", "user.name", "Fixture"])
        self.run_command(["git", "config", "user.email", "fixture@phpwarden.invalid"])
        (self.root / "app/web/modules/custom/demo").mkdir(parents=True)
        (self.root / "app/composer.json").write_text('{"require":{"drupal/core":"^11"}}')
        (self.root / "app/composer.lock").write_text(
            json.dumps(
                {
                    "packages": [
                        {"name": "drupal/core", "version": "11.4.1", "require": {"php": ">=8.3"}}
                    ]
                }
            )
        )
        self.source = self.root / "app/web/modules/custom/demo/example.php"
        self.source.write_text("<?php\n\n/**\n * @file\n * Example.\n */\n\n$example=1;\n")
        shutil.copy2(ROOT / "examples/compose/phpwarden", self.root / "phpwarden")
        compose = (ROOT / "examples/compose/phpwarden.compose.yaml").read_text()
        binary = os.environ.get("PHPWARDEN_TEST_BINARY")
        if binary:
            mount = (
                "\n      - type: bind\n"
                f"        source: {Path(binary).resolve()}\n"
                "        target: /usr/local/bin/phpwarden\n"
                "        read_only: true\n"
            )
            compose = compose.replace("\n\n  phpwarden", mount + "\n  phpwarden") + mount
        (self.root / ".phpwarden.compose.yaml").write_text(compose)
        (self.root / ".phpwarden-image").write_text(self.image + "\n")
        self.run_command(["git", "add", "app"])
        self.run_command(["git", "commit", "-qm", "Fixture"])

    def run_command(self, arguments, expected=0, cwd=None):
        result = subprocess.run(
            [str(x) for x in arguments],
            cwd=cwd or self.root,
            env=self.environment,
            capture_output=True,
            text=True,
            timeout=180,
        )
        if result.returncode != expected:
            self.fail(f"{arguments}: {result.returncode}\n{result.stdout}\n{result.stderr}")
        return result

    def init(self):
        self.run_command(
            [
                "./phpwarden",
                "init",
                "-n",
                "--php-runtime",
                "8.3",
                "--composer-root",
                "app",
                "--pre-commit",
                "--agents",
                "all",
            ]
        )

    def test_init_check_fix_and_host_precommit_in_monorepo(self):
        self.init()
        configuration = self.root / ".phpwarden.yaml"
        before = configuration.read_bytes()
        self.init()
        self.assertEqual(before, configuration.read_bytes())
        self.assertEqual(configuration.stat().st_uid, os.getuid())
        self.assertIn(
            "\"$project_root\"/'phpwarden'", (self.root / ".git/hooks/pre-commit").read_text()
        )
        check = self.run_command(
            [
                "./phpwarden",
                "check",
                str(self.source.relative_to(self.root)),
                "--only",
                "debug,syntax",
                "--format=json",
            ]
        )
        self.assertTrue(json.loads(check.stdout)["complete"])
        result = self.run_command(
            [
                "./phpwarden",
                "fix",
                str(self.source.relative_to(self.root)),
                "--only",
                "phpcs",
                "--format=json",
            ]
        )
        self.assertIn(
            str(self.source.relative_to(self.root)), json.loads(result.stdout)["fixed_files"]
        )
        self.assertIn("$example = 1;", self.source.read_text())
        effective = json.loads(
            self.run_command(
                [
                    "./phpwarden",
                    "config",
                    "show",
                    "--format=json",
                ]
            ).stdout
        )["effective"]["config"]
        configuration.write_text(
            json.dumps(
                {
                    "version": 1,
                    "profile": "drupal",
                    "composer_root": "app",
                    "php": effective["php"],
                    "tools": {name: {"enabled": name == "debug"} for name in effective["tools"]},
                }
            )
        )
        self.source.write_text("<?php\nvar_dump('fixture');\n")
        self.run_command(["git", "add", "app"])
        blocked = self.run_command([".git/hooks/pre-commit"], expected=1)
        self.assertIn("Correctness", blocked.stdout)
        self.source.write_text("<?php\n$example = 1;\n")
        self.run_command(["git", "add", "app"])
        self.run_command([".git/hooks/pre-commit"])
        self.source.write_text("<?php\n$example = 2;\n")
        self.run_command([".git/hooks/pre-commit"], expected=2)

    def test_runtime_mismatch_and_foreign_hook_are_not_silent(self):
        self.init()
        hook = self.root / ".git/hooks/pre-commit"
        hook.write_text("#!/bin/sh\nexit 17\n")
        self.run_command(["./phpwarden", "init", "-n", "--pre-commit"], expected=2)
        self.assertEqual(hook.read_text(), "#!/bin/sh\nexit 17\n")
        config = self.root / ".phpwarden.yaml"
        config.write_text(config.read_text().replace('runtime: "8.3"', 'runtime: "8.4"'))
        result = self.run_command(
            [
                "./phpwarden",
                "check",
                str(self.source.relative_to(self.root)),
                "--only",
                "syntax",
                "--format=json",
            ],
            expected=3,
        )
        self.assertFalse(json.loads(result.stdout)["complete"])

    def test_linked_worktree_uses_common_git_metadata(self):
        self.init()
        effective = json.loads(
            self.run_command(
                [
                    "./phpwarden",
                    "config",
                    "show",
                    "--format=json",
                ]
            ).stdout
        )["effective"]["config"]
        (self.root / ".phpwarden.yaml").write_text(
            json.dumps(
                {
                    "version": 1,
                    "profile": "drupal",
                    "composer_root": "app",
                    "php": effective["php"],
                    "tools": {name: {"enabled": name == "debug"} for name in effective["tools"]},
                }
            )
        )
        self.run_command(
            [
                "git",
                "add",
                ".phpwarden.yaml",
                "phpwarden",
                ".phpwarden.compose.yaml",
                ".phpwarden-image",
            ]
        )
        self.run_command(["git", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "Integration"])
        worktree = Path(tempfile.mkdtemp(prefix="phpwarden-linked-test-"))
        self.run_command(["git", "worktree", "add", "--quiet", "-b", "fixture-worktree", worktree])
        self.addCleanup(shutil.rmtree, worktree, True)
        result = self.run_command(
            [
                "./phpwarden",
                "check",
                "app/web/modules/custom/demo/example.php",
                "--only=syntax,debug",
                "--format=json",
            ],
            cwd=worktree,
        )
        self.assertTrue(json.loads(result.stdout)["complete"])
        source = worktree / "app/web/modules/custom/demo/example.php"
        source.write_text("<?php\nvar_dump('linked fixture');\n")
        self.run_command(["git", "add", "app"], cwd=worktree)
        self.run_command([self.root / ".git/hooks/pre-commit"], expected=1, cwd=worktree)


if __name__ == "__main__":
    unittest.main()
