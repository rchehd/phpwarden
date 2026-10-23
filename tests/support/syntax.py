import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "syntax"


def check_syntax_fixtures(test, configure, run):
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
                test.assertEqual(result["exit_code"], 1, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                actual = [(row["checker"], row["path"], row["line"]) for row in result["findings"]]
                expected = [
                    (row["checker"], row["path"], row["line"]) for row in scenario.spec["findings"]
                ]
                test.assertEqual(actual, expected, result)
                test.assertTrue(all(row["kind"] == "syntax" for row in result["findings"]))
                test.assertEqual(
                    [row["id"] for row in result["checks"]],
                    ["debug", "drupal-config", "phpcs", "syntax", "whitespace", "yaml"],
                )
                if mode == "diff" and directory.name == "deletion-syntax-error":
                    test.assertEqual(
                        result["scope"]["line_map"][scenario.spec["changed_files"][0]], []
                    )


def check_syntax_settings(test, configure, run):
    with materialize(FIXTURES / "added-syntax-error") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {
            "debug": {"enabled": False},
            "phpcs": {"enabled": False},
            "drupal-config": {"enabled": False},
            "syntax": {"enabled": False},
            "yaml": {"enabled": False},
            "whitespace": {"enabled": False},
        }
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        result = run(scenario, ["--only", "syntax"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["id"] for row in result["checks"]], ["syntax"])
        test.assertEqual(run(scenario, ["--only", "debug"])["exit_code"], 0)
        test.assertEqual(run(scenario, ["--only", "syntax,debug"])["exit_code"], 1)
        for value in ("syntax,syntax", "syntax,phpstna", "syntax,"):
            test.assertEqual(run(scenario, ["--only", value])["exit_code"], 2)
        config["tools"]["syntax"]["fail_on"] = "never"
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "syntax"])
        test.assertEqual(result["exit_code"], 0)
        test.assertEqual(len(result["findings"]), 1)
        test.assertTrue(result["complete"])
        config["tools"]["syntax"] = {"enabled": True}
        filename.write_text(json.dumps(config))
        target = scenario.root / scenario.spec["changed_files"][0]
        target.write_text("<?php\nfile_put_contents(__DIR__ . '/executed', 'bad');\n")
        extra = target.with_name("template.phtml")
        extra.write_text("<?php\n$value = ;\n")
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        test.assertFalse(target.with_name("executed").exists())
        config["tools"]["syntax"]["extra_extensions"] = [".phtml"]
        filename.write_text(json.dumps(config))
        result = run(scenario, [])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual(result["findings"][0]["path"], str(extra.relative_to(scenario.root)))
        config["tools"]["syntax"]["exclude"] = ["**/*.phtml"]
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
