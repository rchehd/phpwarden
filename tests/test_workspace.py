import json
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from tools.sandbox.run import Sandbox, main
from tools.sandbox.workspace import finish, remove


class WorkspaceTest(unittest.TestCase):
    def test_readonly_projects_removed_without_following_links_or_deleting_evidence(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            root, outside = parent / "output", parent / "original"
            root.mkdir()
            outside.mkdir()
            (outside / "keep").write_text("original")
            site = root / "project/sites/default"
            site.mkdir(parents=True)
            (site / "settings.php").write_text("fixture")
            site.chmod(0o555)
            (root / "project/vendor").symlink_to(outside, target_is_directory=True)
            (root / "cache").symlink_to(outside, target_is_directory=True)
            (root / "summary.json").write_text("evidence")
            result = remove(root, ("project", "cache"))
            self.assertEqual(result["status"], "removed")
            self.assertFalse((root / "project").exists())
            self.assertFalse((root / "cache").is_symlink())
            self.assertEqual((outside / "keep").read_text(), "original")
            self.assertEqual((root / "summary.json").read_text(), "evidence")

    def test_invalid_targets_rejected_before_any_deletion(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "project").mkdir()
            for invalid in ("..", ".", "", "../original", "/tmp", "project/vendor"):
                with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                    remove(root, ("project", invalid))
                self.assertTrue((root / "project").is_dir())

    def test_active_mount_is_retained(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "project/vendor").mkdir(parents=True)
            for mount in (root / "project/vendor", root, root.parent):
                result = remove(root, ("project",), mounts=[mount])
                self.assertEqual(
                    result["errors"], [{"path": "project", "error_type": "ActiveMount"}]
                )
            self.assertTrue((root / "project/vendor").is_dir())

    def test_failed_resource_inspection_preserves_workspace(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "project").mkdir()
            sandbox = SimpleNamespace(root=root, summary={"passed": True}, keep_workspace=False)
            with patch(
                "tools.sandbox.workspace.active_sources", side_effect=ValueError("bad JSON")
            ):
                with self.assertRaisesRegex(RuntimeError, "Cannot verify workspace resources"):
                    finish(sandbox, ("project",), check_mounts=True)
            self.assertTrue((root / "project").is_dir())
            self.assertFalse(sandbox.summary["passed"])
            self.assertEqual(sandbox.summary["workspace_cleanup"]["status"], "failed")

    def test_failed_packaging_cleans_only_new_output(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "output"

            def fail(version, commit, manifest, output):
                output.mkdir()
                (output / "partial-archive").write_text("partial")
                raise RuntimeError("package failed")

            arguments = ["sandbox", "--drupal", "11", "--output", str(root)]
            with patch("sys.argv", arguments), patch("tools.sandbox.run.package", fail):
                with self.assertRaisesRegex(RuntimeError, "package failed"):
                    main()
            self.assertFalse((root / "dist").exists())
            self.assertFalse(json.loads((root / "summary.json").read_text())["passed"])
            sentinel = root / "project"
            sentinel.mkdir()
            (sentinel / "keep").write_text("existing")
            with patch("sys.argv", arguments), self.assertRaises(FileExistsError):
                main()
            self.assertEqual((sentinel / "keep").read_text(), "existing")

    def test_failure_and_interrupt_cleanup_preserve_reports(self):
        for error in (RuntimeError("check failed"), KeyboardInterrupt()):
            with (
                self.subTest(error=type(error).__name__),
                tempfile.TemporaryDirectory() as directory,
            ):
                sandbox = Sandbox("11", Path(directory) / "output", "worker", Path("archive"), 1)
                (sandbox.root / "unit-project/vendor").mkdir(parents=True)
                stopped = []
                sandbox.prepare_source = lambda: None

                def fail(error=error):
                    raise error

                sandbox.install = fail
                sandbox.cleanup_installer = lambda stopped=stopped: stopped.append(True)
                with patch("tools.sandbox.workspace.active_sources", return_value=[]):
                    with self.assertRaises(type(error)):
                        sandbox.execute()
                self.assertEqual(stopped, [True])
                self.assertFalse(sandbox.project.exists())
                self.assertFalse((sandbox.root / "unit-project").exists())
                summary = json.loads((sandbox.root / "summary.json").read_text())
                self.assertFalse(summary["passed"])
                self.assertEqual(summary["workspace_cleanup"]["status"], "removed")
                self.assertTrue((sandbox.root / "evidence/index.json").is_file())

    def test_explicit_keep_and_failed_resource_cleanup_preserve_workspace(self):
        for resource_failure in (False, True):
            with (
                self.subTest(resource_failure=resource_failure),
                tempfile.TemporaryDirectory() as directory,
            ):
                sandbox = Sandbox(
                    "10",
                    Path(directory) / "output",
                    "worker",
                    Path("archive"),
                    1,
                    keep_workspace=not resource_failure,
                )
                sandbox.prepare_source = lambda: None

                def fail():
                    raise RuntimeError("fixture failure")

                sandbox.install = fail
                sandbox.cleanup_installer = fail if resource_failure else lambda: None
                with self.assertRaises(RuntimeError):
                    sandbox.execute()
                self.assertTrue(sandbox.project.is_dir())
                summary = json.loads((sandbox.root / "summary.json").read_text())
                self.assertFalse(summary["passed"])
                self.assertEqual(summary["workspace_cleanup"]["status"], "retained")


if __name__ == "__main__":
    unittest.main()
