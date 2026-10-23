import json
from pathlib import Path

from tests.support.phpmd import TOOLS
from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "gitleaks"


def check_gitleaks_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                filename = scenario.root / ".phpwarden.yaml"
                config = json.loads(filename.read_text())
                config["tools"] = {name: {"enabled": False} for name in TOOLS} | {
                    "gitleaks": scenario.spec["settings"] | {"enabled": True}
                }
                filename.write_text(json.dumps(config))
                args = {
                    "diff": [],
                    "full": ["--mode", "full"],
                    "whole-files": ["--whole-files"],
                    "paths": scenario.spec["changed_files"],
                }[mode]
                result = run(scenario, args)
                expected = sorted(
                    (row["path"], row["line"])
                    for row in scenario.spec["findings"]
                    if ("diff" if mode == "diff" else "full") in row["modes"]
                    and (
                        mode not in ("paths", "whole-files")
                        or row["path"] in scenario.spec["changed_files"]
                    )
                )
                actual = sorted((row["path"], row["line"]) for row in result["findings"])
                test.assertEqual(actual, expected, result)
                test.assertEqual(result["exit_code"], 1, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [], result)
                for row in result["findings"]:
                    test.assertEqual(row["checker"], "gitleaks", row)
                    test.assertEqual(row["severity"], "error", row)
                    test.assertIn("redacted", row["message"], row)
                for row in scenario.spec["findings"]:
                    test.assertNotIn(row["contains"], json.dumps(result))


def check_strict_rules(test, configure, run):
    with materialize(FIXTURES / "gitleaks-unstaged") as scenario:
        configure(scenario)
        name = "web/modules/custom/demo/compare.php"
        (scenario.root / name).write_text(
            "<?php\nfunction compare(int $left, int $right): bool {\n"
            "    return $left == $right;\n}\n"
        )
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        for enabled, expected in ((False, 0), (True, 1)):
            config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
                "phpstan": {
                    "enabled": True,
                    "drupal": False,
                    "strict_rules": {
                        "enabled": enabled,
                        "all": False,
                        "rules": {"disallowedLooseComparison": True},
                    },
                }
            }
            filename.write_text(json.dumps(config))
            result = run(scenario, [name])
            test.assertEqual(len(result["findings"]), expected, result)
            test.assertEqual(result["exit_code"], 1 if expected else 0, result)
            test.assertTrue(result["complete"], result)
            test.assertEqual(result["diagnostics"], [], result)
