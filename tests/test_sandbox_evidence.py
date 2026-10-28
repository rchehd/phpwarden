import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tools.sandbox.assertions import preserved, tree_snapshot
from tools.sandbox.commands import Commands
from tools.sandbox.evidence import Evidence
from tools.sandbox.report import render


class EvidenceTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.sandbox = Commands()
        self.sandbox.root = self.root
        self.sandbox.project = self.root / "project"
        self.sandbox.project.mkdir()
        self.sandbox.logs = self.root / "logs"
        self.sandbox.logs.mkdir()
        self.sandbox.sequence = 0
        self.sandbox.environment = {}
        self.sandbox.summary = {"drupal_major": "11", "environment": "host", "passed": True}
        self.sandbox.evidence = Evidence(self.sandbox)

    def test_cleanup_failure_stays_red_and_other_cleanup_steps_still_run(self):
        sandbox = self.sandbox
        with sandbox.case("install", "npm"):
            pass
        completed = []

        def fail():
            raise RuntimeError("private fixture diagnostic")

        errors = sandbox.cleanup_steps(
            [("runtime", fail), ("alias", lambda: completed.append(True))]
        )
        self.assertEqual(completed, [True])
        self.assertEqual(errors[0]["step"], "runtime")
        sandbox.summary["passed"] = True
        sandbox.evidence.finalize()
        self.assertFalse(sandbox.summary["passed"])
        (self.root / "summary.json").write_text(json.dumps(sandbox.summary))
        text, passed = render([self.root])
        self.assertFalse(passed)
        self.assertIn("**FAIL**", text)
        self.assertIn("| npm | install | passed |", text)
        self.assertIn("cleanup-runtime | failed", text)
        result = self.root / "evidence/11/host/shared/cleanup-runtime/result.json"
        self.assertNotIn("private fixture diagnostic", result.read_text())

    def test_timeout_keeps_partial_logs_in_the_method_case(self):
        sandbox = self.sandbox
        timeout = subprocess.TimeoutExpired(
            ["fixture", "private-token"], 1, output=b"partial stdout", stderr=b"partial stderr"
        )
        with self.assertRaises(subprocess.TimeoutExpired), sandbox.case("install", "composer"):
            with patch("tools.sandbox.commands.subprocess.run", side_effect=timeout):
                sandbox.run(["fixture", "private-token"], timeout=1)
        path = self.root / "evidence/11/host/composer/install"
        self.assertEqual((path / "logs/001.stdout").read_text(), "partial stdout")
        record = json.loads((path / "result.json").read_text())
        self.assertEqual(record["status"], "failed")
        self.assertTrue(record["commands"][0]["timed_out"])
        self.assertNotIn("private-token", json.dumps(record))

    def test_preservation_detects_changes_including_failure_paths(self):
        file = self.sandbox.project / "config"
        absent = self.sandbox.project / "absent"
        for change in ("content", "mode", "created", "removed"):
            file.write_text("preserve me")
            file.chmod(0o600)
            absent.unlink(missing_ok=True)
            with self.subTest(change=change), self.assertRaises(AssertionError):
                with preserved([file, absent]):
                    if change == "content":
                        file.write_text("changed")
                    elif change == "mode":
                        file.chmod(0o644)
                    elif change == "created":
                        absent.touch()
                    else:
                        file.unlink()
        file.write_text("unchanged")
        with self.assertRaisesRegex(RuntimeError, "original failure"):
            with preserved([file]):
                raise RuntimeError("original failure")

    def test_tree_snapshot_includes_symlink_identity_without_following_it(self):
        source = self.sandbox.project / "source"
        source.write_text("data")
        link = self.sandbox.project / "link"
        link.symlink_to("source")
        before = tree_snapshot(self.sandbox.project)
        self.assertEqual(before["link"]["link"], "source")
        link.unlink()
        link.symlink_to("different-target")
        self.assertNotEqual(before, tree_snapshot(self.sandbox.project))

    def test_evidence_requires_valid_components_and_missing_reports_fail(self):
        with self.assertRaises(ValueError), self.sandbox.case("../../escape"):
            self.fail("Invalid case name was accepted")
        self.sandbox.evidence.finalize()
        self.assertFalse(self.sandbox.summary["passed"])
        text, passed = render([self.root / "missing|<report>"])
        self.assertFalse(passed)
        self.assertIn("missing&#124;&lt;report&gt;", text)


if __name__ == "__main__":
    unittest.main()
