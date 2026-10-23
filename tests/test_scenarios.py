import unittest
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).parent / "fixtures" / "scenarios"


class ScenarioFixturesTest(unittest.TestCase):
    def scenarios(self):
        directories = sorted(
            path
            for group in (
                FIXTURES,
                FIXTURES.parent / "syntax",
                FIXTURES.parent / "yaml",
                FIXTURES.parent / "whitespace",
                FIXTURES.parent / "drupal-config",
                FIXTURES.parent / "phpcs",
                FIXTURES.parent / "phpstan",
                FIXTURES.parent / "twig-cs-fixer",
                FIXTURES.parent / "composer-validate",
            )
            for path in group.iterdir()
            if path.is_dir()
        )
        self.assertGreater(len(directories), 0, "No scenario fixtures found")
        return directories

    def test_changed_files_match_scenario_manifest(self):
        for directory in self.scenarios():
            with self.subTest(scenario=directory.name), materialize(directory) as scenario:
                self.assertEqual(scenario.changed_files(), scenario.spec["changed_files"])

    def test_git_state_matches_scenario_manifest(self):
        for directory in self.scenarios():
            with self.subTest(scenario=directory.name), materialize(directory) as scenario:
                status = scenario.git("status", "--porcelain=v1", "-z")
                state = scenario.spec["state"]
                if state == "committed":
                    self.assertEqual(status, b"")
                    self.assertNotEqual(
                        scenario.git("rev-parse", "HEAD").decode().strip(), scenario.base
                    )
                else:
                    prefix = {"unstaged": b" M ", "staged": b"M  ", "untracked": b"?? "}[state]
                    self.assertTrue(status.startswith(prefix), status)
                self.assertEqual(scenario.git("rev-parse", "main").decode().strip(), scenario.base)

    def test_expected_findings_point_to_fixture_content(self):
        for directory in self.scenarios():
            with self.subTest(scenario=directory.name), materialize(directory) as scenario:
                for finding in scenario.spec["findings"]:
                    path = (scenario.root / finding["path"]).resolve()
                    self.assertTrue(path.is_relative_to(scenario.root))
                    if finding.get("global"):
                        self.assertIn(
                            finding["checker"], ("yaml", "twig-cs-fixer", "composer-validate")
                        )
                        self.assertIsNone(finding["line"])
                        self.assertTrue(path.is_file())
                        continue
                    lines = path.read_text().splitlines()
                    self.assertGreater(finding["line"], 0)
                    if finding.get("eof"):
                        self.assertIn(finding["checker"], ("syntax", "yaml"))
                        self.assertEqual(finding["line"], len(lines) + 1)
                        continue
                    self.assertLessEqual(finding["line"], len(lines))
                    self.assertIn(finding["contains"], lines[finding["line"] - 1])

    def test_verdicts_are_consistent_with_declared_findings(self):
        for directory in self.scenarios():
            with self.subTest(scenario=directory.name), materialize(directory) as scenario:
                for mode in ("diff", "full"):
                    findings = [row for row in scenario.spec["findings"] if mode in row["modes"]]
                    expected = "fail" if findings else "pass"
                    self.assertEqual(scenario.spec["verdicts"][mode], expected)

    def test_scenarios_are_isolated_and_removed_after_use(self):
        directory = FIXTURES / "added-debug"
        with materialize(directory) as first, materialize(directory) as second:
            self.assertNotEqual(first.root, second.root)
            (first.root / "local-marker").write_text("first scenario only\n")
            self.assertFalse((second.root / "local-marker").exists())
            roots = (first.root, second.root)
        for root in roots:
            self.assertFalse(root.exists())

    def test_unusual_filename_is_created_only_in_temporary_repository(self):
        directory = FIXTURES.parent / "phpcs" / "phpcs-untracked-name"
        portable = "web/modules/custom/demo/new-file.module"
        unusual = "web/modules/custom/demo/new \nfile.module"
        self.assertTrue((directory / "untracked" / portable).is_file())
        self.assertFalse((directory / "untracked" / unusual).exists())
        with materialize(directory) as scenario:
            self.assertFalse((scenario.root / portable).exists())
            self.assertEqual(
                (scenario.root / unusual).read_bytes(),
                (directory / "untracked" / portable).read_bytes(),
            )
            self.assertEqual(scenario.changed_files(), [unusual])
            root = scenario.root
        self.assertFalse(root.exists())


if __name__ == "__main__":
    unittest.main()
