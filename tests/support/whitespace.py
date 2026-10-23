import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "whitespace"


def check_whitespace_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                arguments = {
                    "diff": [],
                    "full": ["--mode", "full"],
                    "whole-files": ["--whole-files"],
                    "paths": scenario.spec["changed_files"],
                }[mode]
                result = run(scenario, arguments)
                expected_mode = "diff" if mode == "diff" else "full"
                expected = sorted(
                    (row["path"], row["line"], row["rule"])
                    for row in scenario.spec["findings"]
                    if expected_mode in row["modes"]
                )
                test.assertEqual(
                    sorted((row["path"], row["line"], row["rule"]) for row in result["findings"]),
                    expected,
                    result,
                )
                test.assertEqual(result["exit_code"], 1 if expected else 0, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                test.assertEqual(
                    [row["id"] for row in result["checks"]],
                    ["debug", "drupal-config", "phpcs", "syntax", "whitespace", "yaml"],
                )
                for row in result["findings"]:
                    test.assertEqual(row["checker"], "whitespace")
                    test.assertEqual(row["kind"], "code")
                if mode == "diff" and directory.name == "whitespace-deletion-only":
                    test.assertEqual(
                        result["scope"]["line_map"][scenario.spec["changed_files"][0]], []
                    )


def check_whitespace_settings(test, configure, run):
    with materialize(FIXTURES / "whitespace-markdown-conflict") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {
            name: {"enabled": False}
            for name in ("debug", "drupal-config", "phpcs", "syntax", "whitespace", "yaml")
        }
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        result = run(scenario, ["--only", "whitespace"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["id"] for row in result["checks"]], ["whitespace"])
        test.assertEqual(len(result["findings"]), 4)
        for settings, count, exit_code in (
            ({"fail_on": "never"}, 4, 0),
            ({"conflict_markers": False}, 0, 0),
            ({"conflict_markers": False, "trailing_whitespace_exclude": []}, 2, 1),
            ({"trailing_whitespace": False, "conflict_markers_exclude": ["**/*.md"]}, 0, 0),
            ({"exclude": ["**/*.md"]}, 0, 0),
            ({"include": ["web/modules/custom/missing.txt"]}, 0, 0),
            ({"trailing_whitespace": False, "conflict_markers": False}, 0, 0),
        ):
            with test.subTest(settings=settings):
                config["tools"]["whitespace"] = settings
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "whitespace"])
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual(len(result["findings"]), count, result)
                test.assertTrue(result["complete"], result)
