import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "phpmd"
TOOLS = (
    "composer-audit",
    "composer-require-checker",
    "composer-unused",
    "composer-validate",
    "dead-code",
    "debug",
    "deptrac",
    "drupal-config",
    "eslint",
    "eventhorizon",
    "gitleaks",
    "jscpd",
    "phpcs",
    "phpmd",
    "phpstan",
    "psalm",
    "rector",
    "semgrep",
    "stylelint",
    "syntax",
    "twig-cs-fixer",
    "whitespace",
    "yaml",
)


def sort_key(row):
    return (row[0], row[1] or "", row[2] or 0)


def only_phpmd(scenario, settings):
    filename = scenario.root / ".phpwarden.yaml"
    config = json.loads(filename.read_text())
    config["tools"] = {name: {"enabled": False} for name in TOOLS} | {"phpmd": settings}
    filename.write_text(json.dumps(config))


def check_phpmd_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                only_phpmd(scenario, scenario.spec["settings"] | {"enabled": True})
                arguments = {
                    "diff": [],
                    "full": ["--mode", "full"],
                    "whole-files": ["--whole-files"],
                    "paths": scenario.spec["changed_files"],
                }[mode]
                result = run(scenario, arguments)
                expected_mode = "diff" if mode == "diff" else "full"
                expected = sorted(
                    (
                        (row["checker"], None if row.get("global") else row["path"], row["line"])
                        for row in scenario.spec["findings"]
                        if expected_mode in row["modes"]
                    ),
                    key=sort_key,
                )
                actual = sorted(
                    ((row["checker"], row["path"], row["line"]) for row in result["findings"]),
                    key=sort_key,
                )
                test.assertEqual(actual, expected, result)
                test.assertEqual(result["exit_code"], 1 if expected else 0, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                for row in result["findings"]:
                    test.assertEqual(row["severity"], "warning", row)
                    if row["line"] is None:
                        test.assertEqual(row["kind"], "global", row)
                        test.assertEqual(row["rule"], "phpmd.parse_error", row)
                        test.assertTrue(row["message"].startswith("PHP file "), row)
                        test.assertNotIn(str(scenario.root), row["message"])


def check_phpmd_settings(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        for settings, rules, code in (
            ({}, ["UnusedLocalVariable"], 0),
            ({"fail_on": "warning"}, ["UnusedLocalVariable"], 1),
            ({"fail_on": "never"}, ["UnusedLocalVariable"], 0),
            ({"exclude_rules": []}, ["UnusedFormalParameter", "UnusedLocalVariable"], 0),
            ({"rulesets": ["naming"], "exclude_rules": []}, [], 0),
            ({"exclude": ["**/*.module"]}, [], 0),
        ):
            with test.subTest(settings=settings):
                only_phpmd(scenario, settings | {"enabled": True})
                result = run(scenario, ["--mode", "full"])
                test.assertEqual(result["exit_code"], code, result)
                test.assertEqual(sorted(row["rule"] for row in result["findings"]), rules, result)
                status = next(row for row in result["checks"] if row["id"] == "phpmd")
                if "exclude" in settings:
                    test.assertEqual(status["reason"], "no_files", result)
        only_phpmd(scenario, {"enabled": False})
        result = run(scenario, ["--only", "phpmd", "--mode", "full"])
        test.assertEqual([row["rule"] for row in result["findings"]], ["UnusedLocalVariable"])
