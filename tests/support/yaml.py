import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "yaml"


def check_yaml_fixtures(test, configure, run):
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
                expected = [
                    (row["checker"], None if row.get("global") else row["path"], row["line"])
                    for row in scenario.spec["findings"]
                ]
                actual = [(row["checker"], row["path"], row["line"]) for row in result["findings"]]
                test.assertEqual(actual, expected, result)
                test.assertEqual(result["exit_code"], 1 if expected else 0, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                test.assertEqual(
                    [row["id"] for row in result["checks"]], ["debug", "syntax", "yaml"]
                )
                for row in result["findings"]:
                    test.assertEqual(row["kind"], "global" if row["line"] is None else "syntax")
                if mode == "diff" and directory.name == "yaml-deletion-error":
                    test.assertEqual(
                        result["scope"]["line_map"][scenario.spec["changed_files"][0]], []
                    )
                test.assertFalse((scenario.root / "vendor").exists())


def check_yaml_settings(test, configure, run):
    with materialize(FIXTURES / "yaml-duplicate-key") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {
            "debug": {"enabled": False},
            "syntax": {"enabled": False},
            "yaml": {"enabled": False},
        }
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        result = run(scenario, ["--only", "yaml"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["id"] for row in result["checks"]], ["yaml"])
        test.assertEqual(run(scenario, ["--only", "debug,syntax"])["exit_code"], 0)
        config["tools"]["yaml"] = {"enabled": True, "fail_on": "never"}
        filename.write_text(json.dumps(config))
        result = run(scenario, [])
        test.assertEqual(result["exit_code"], 0)
        test.assertEqual(len(result["findings"]), 1)
        config["tools"]["yaml"] = {"exclude": ["**/*.yml"]}
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "yaml"])
        test.assertEqual(result["exit_code"], 0)
        test.assertEqual(result["checks"][0]["reason"], "no_files")
        config["tools"]["yaml"] = {"include": ["web/modules/custom/missing.yaml"]}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, ["--only", "yaml"])["exit_code"], 0)
        config["tools"]["yaml"] = {"enabled": True}
        filename.write_text(json.dumps(config))
        target = scenario.root / scenario.spec["changed_files"][0]
        target.write_text("")
        vendor = scenario.root / "vendor"
        vendor.mkdir()
        (vendor / "autoload.php").write_text(
            "<?php file_put_contents(__DIR__ . '/../executed', 'bad');\n"
        )
        test.assertEqual(run(scenario, ["--only", "yaml"])["exit_code"], 0)
        test.assertFalse((scenario.root / "executed").exists())
