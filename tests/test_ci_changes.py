import json
import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path

from tools.ci.changes import GROUPS, changed_paths, classify

ROOT = Path(__file__).resolve().parents[1]


class CheckSelectionTest(unittest.TestCase):
    def selected(self, *paths):
        return {name for name, enabled in classify(paths).items() if enabled}

    def test_docs_do_not_start_tool_installations_or_images(self):
        self.assertEqual(
            self.selected("README.md", "docs/TESTING.md", "tools/mutation/README.md"), set()
        )

    def test_pure_unit_tests_and_python_harness_select_their_own_checks(self):
        self.assertEqual(self.selected("internal/projecttest/mutation_test.go"), {"go"})
        self.assertEqual(self.selected("tests/test_sandbox.py"), {"python"})
        self.assertEqual(self.selected("requirements-dev.txt"), {"python"})
        self.assertEqual(self.selected("pyproject.toml"), {"python"})

    def test_mutation_changes_do_not_start_unrelated_installation_scenarios(self):
        self.assertEqual(self.selected("tools/sandbox/mutation_tests.py"), {"python", "mutation"})
        selected = self.selected("internal/projecttest/mutation.go")
        self.assertIn("go", selected)
        self.assertIn("sandbox", selected)
        self.assertIn("mutation", selected)

    def test_embedded_markdown_and_text_fixtures_are_not_treated_as_docs(self):
        self.assertEqual(self.selected("tests/fixtures/sample/keep.txt"), {"go", "python"})
        self.assertEqual(self.selected("tests/fixtures/sample/guide.md"), {"go", "python"})
        self.assertIn("dockerless", self.selected("internal/agentskills/assets/review/SKILL.md"))

    def test_worker_build_and_shared_scope_dependencies_select_all_consumers(self):
        for path in (
            "docker/Dockerfile",
            "docker-bake.json",
            "go.sum",
            "schemas/config-v1.schema.json",
            "tools/ci/changes.py",
            ".github/actions/changes/action.yml",
        ):
            with self.subTest(path=path):
                self.assertEqual(self.selected(path), set(GROUPS))
        for path in ("tools/phpstan/composer.lock", "tools/mutation/prepare.php"):
            with self.subTest(path=path):
                self.assertTrue({"go", "sandbox", "mutation"} <= self.selected(path))
        self.assertIn("mutation", self.selected("tools/sandbox/project_tests.py"))
        self.assertIn("dockerless", self.selected("tools/sandbox/mcp.py"))

    def test_manual_and_reusable_validations_can_force_all_groups(self):
        self.assertTrue(all(classify(["README.md"], force=True).values()))
        paths, _ = changed_paths({}, "workflow_dispatch", ROOT)
        self.assertIsNone(paths)


class GitComparisonTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="phpwarden-ci-selection-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.environment = os.environ | {
            "GIT_CONFIG_GLOBAL": "/dev/null",
            "GIT_CONFIG_NOSYSTEM": "1",
        }
        self.git("init", "--quiet", "--initial-branch=main")
        self.write("README.md", "base\n")
        self.write("cmd/main.go", "base\n")
        self.base = self.commit()

    def git(self, *arguments):
        return subprocess.run(
            [
                "git",
                "-c",
                "user.name=Fixture",
                "-c",
                "user.email=fixture@localhost",
                "-c",
                "core.hooksPath=/dev/null",
                "-c",
                "commit.gpgSign=false",
                *arguments,
            ],
            cwd=self.root,
            env=self.environment,
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()

    def write(self, name, value):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(value)

    def commit(self):
        self.git("add", "--all")
        self.git("commit", "--quiet", "-m", "Fixture change")
        return self.git("rev-parse", "HEAD")

    def test_pr_uses_merge_base_and_ignores_changes_only_on_target_branch(self):
        self.git("checkout", "--quiet", "-b", "feature")
        self.write("README.md", "PR docs\n")
        head = self.commit()
        self.git("checkout", "--quiet", "main")
        self.write("cmd/main.go", "target-only change\n")
        base = self.commit()
        paths, _ = changed_paths(
            {"pull_request": {"base": {"sha": base}, "head": {"sha": head}}},
            "pull_request",
            self.root,
        )
        self.assertEqual(paths, ["README.md"])
        self.assertFalse(any(classify(paths).values()))

    def test_push_covers_every_commit_and_keeps_deleted_rename_source(self):
        self.write("internal/projecttest/old.go", "old\n")
        base = self.commit()
        (self.root / "docs").mkdir()
        self.git("mv", "internal/projecttest/old.go", "docs/moved.md")
        self.commit()
        self.write("tests/new\nname.py", "value\n")
        head = self.commit()
        paths, _ = changed_paths({"before": base, "after": head}, "push", self.root)
        self.assertIn("internal/projecttest/old.go", paths)
        self.assertIn("docs/moved.md", paths)
        self.assertIn("tests/new\nname.py", paths)
        self.assertTrue(classify(paths)["mutation"])
        self.assertTrue(classify(paths)["python"])

    def test_git_comparison_is_not_limited_to_first_300_files(self):
        for index in range(320):
            self.write(f"docs/page{index}.md", "docs\n")
        self.write("internal/projecttest/last.go", "source\n")
        head = self.commit()
        paths, _ = changed_paths({"before": self.base, "after": head}, "push", self.root)
        self.assertEqual(len(paths), 321)
        self.assertTrue(classify(paths)["mutation"])

    def test_missing_history_runs_all_and_invalid_event_fails(self):
        for base in ("0" * 40, "f" * 40):
            paths, _ = changed_paths({"before": base, "after": self.base}, "push", self.root)
            self.assertIsNone(paths)
        with self.assertRaises(ValueError):
            changed_paths({"before": "--invalid", "after": self.base}, "push", self.root)
        self.assertEqual(changed_paths({"deleted": True}, "push", self.root)[0], [])

    def test_cli_emits_boolean_action_outputs_for_docs_and_manual_override(self):
        output = self.root / "outputs"
        environment = self.environment | {"GITHUB_OUTPUT": str(output), "CI_FORCE_ALL": "false"}
        command = [sys.executable, str(ROOT / "tools/ci/changes.py"), "--paths", "README.md"]
        result = subprocess.run(
            command, env=environment, capture_output=True, text=True, check=True
        )
        self.assertFalse(any(json.loads(result.stdout)["selected"].values()))
        self.assertEqual(set(output.read_text().splitlines()), {f"{name}=false" for name in GROUPS})
        output.unlink()
        environment["CI_FORCE_ALL"] = "true"
        subprocess.run(command, env=environment, capture_output=True, text=True, check=True)
        self.assertEqual(set(output.read_text().splitlines()), {f"{name}=true" for name in GROUPS})


class CheckGateTest(unittest.TestCase):
    def test_plugin_only_build_accepts_no_image_publication(self):
        workflow = (ROOT / ".github/workflows/build.yml").read_text()
        script = textwrap.dedent(
            workflow.split("- name: Validate target and publication settings", 1)[1]
            .split("run: |", 1)[1]
            .split("\n  checks:", 1)[0]
        )
        for target, publish, branch, allowed in (
            ("plugins", "false", "main", True),
            ("plugins", "false", "feature/plugins", True),
            ("plugins", "true", "main", False),
            ("cli", "true", "main", False),
            ("images", "true", "main", True),
            ("all", "true", "feature/plugins", False),
            ("unknown", "false", "main", False),
        ):
            with self.subTest(target=target, publish=publish, branch=branch):
                result = subprocess.run(
                    ["bash", "-e", "-c", script],
                    capture_output=True,
                    env=os.environ
                    | {
                        "EVENT_NAME": "workflow_dispatch",
                        "REF_TYPE": "branch",
                        "REF_NAME": branch,
                        "BUILD_TARGET": target,
                        "PUBLISH_IMAGES": publish,
                    },
                )
                self.assertEqual(result.returncode == 0, allowed, result.stderr)

    def test_gate_accepts_intentional_skips_and_rejects_selected_failures(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        script = textwrap.dedent(
            workflow.split("- name: Verify selected checks succeeded", 1)[1].split("run: |", 1)[1]
        )
        for changes, go_required, go_result, python_required, python_result, success in (
            ("success", "false", "skipped", "false", "skipped", True),
            ("success", "true", "success", "false", "skipped", True),
            ("success", "true", "failure", "false", "skipped", False),
            ("success", "true", "skipped", "false", "skipped", False),
            ("success", "false", "skipped", "true", "cancelled", False),
            ("failure", "false", "skipped", "false", "skipped", False),
        ):
            with self.subTest(changes=changes, go=go_result, python=python_result):
                result = subprocess.run(
                    ["bash", "-e", "-c", script],
                    capture_output=True,
                    env=os.environ
                    | {
                        "CHANGES_RESULT": changes,
                        "GO_REQUIRED": go_required,
                        "GO_RESULT": go_result,
                        "PYTHON_REQUIRED": python_required,
                        "PYTHON_RESULT": python_result,
                    },
                )
                self.assertEqual(result.returncode == 0, success)
