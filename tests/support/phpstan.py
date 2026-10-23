import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "phpstan"
TOOLS = ("debug", "drupal-config", "phpcs", "phpstan", "syntax", "whitespace", "yaml")


def check_phpstan_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                filename = scenario.root / ".phpwarden.yaml"
                config = json.loads(filename.read_text())
                config.setdefault("tools", {})["phpstan"] = scenario.spec["settings"] | {
                    "enabled": True
                }
                filename.write_text(json.dumps(config))
                arguments = {
                    "diff": [],
                    "full": ["--mode", "full"],
                    "whole-files": ["--whole-files"],
                    "paths": scenario.spec["changed_files"],
                }[mode]
                result = run(scenario, arguments)
                expected_mode = "diff" if mode == "diff" else "full"
                expected = sorted(
                    (row["checker"], row["path"], row["line"])
                    for row in scenario.spec["findings"]
                    if expected_mode in row["modes"]
                )
                actual = sorted(
                    (row["checker"], row["path"], row["line"]) for row in result["findings"]
                )
                test.assertEqual(actual, expected, result)
                test.assertEqual(result["exit_code"], 1 if expected else 0, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                test.assertEqual([row["id"] for row in result["checks"]], list(TOOLS))
                for row in result["findings"]:
                    test.assertEqual(row["kind"], "code")
                    test.assertEqual(row["severity"], "error")
                    test.assertTrue(row["rule"] and "." in row["rule"], row)
                test.assertEqual(list(scenario.root.rglob("executed")), [])


def check_phpstan_settings(test, configure, run):
    with materialize(FIXTURES / "phpstan-type-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {name: {"enabled": False} for name in TOOLS}
        base = {"drupal": False}
        config["tools"]["phpstan"] = base | {"enabled": False}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        test.assertEqual(run(scenario, ["--only", "phpstan"])["exit_code"], 1)
        for options, count, exit_code in (
            ({"fail_on": "never"}, 1, 0),
            ({"level": 0}, 0, 0),
            ({"exclude": ["**/*.module"]}, 0, 0),
            ({"include": ["web/modules/custom/missing"]}, 0, 0),
        ):
            with test.subTest(options=options):
                config["tools"]["phpstan"] = base | options
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "phpstan"])
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual(len(result["findings"]), count, result)
        for required in (True, False):
            config["tools"]["phpstan"] = base | {
                "config": "missing.neon",
                "required": required,
                "fail_on": "never",
            }
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "phpstan"])
            test.assertEqual(result["exit_code"], 3 if required else 0, result)
            test.assertFalse(result["complete"])
        for required in (True, False):
            config["tools"]["phpstan"] = {"drupal": True, "required": required}
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "phpstan"])
            test.assertEqual(result["exit_code"], 3 if required else 0, result)
            test.assertFalse(result["complete"])
            test.assertEqual(
                [row["status"] for row in result["checks"] if row["id"] == "phpstan"],
                ["unavailable"],
                result,
            )
            test.assertIn(
                "project_dependencies_missing", [row["code"] for row in result["diagnostics"]]
            )
        baseline = scenario.root / "quality/baseline.neon"
        baseline.parent.mkdir()
        baseline.write_text(
            "parameters:\n"
            "  ignoreErrors:\n"
            "    -\n"
            "      message: '#^Function demo_value\\(\\) should return int "
            "but returns string\\.$#'\n"
            "      identifier: return.type\n"
            "      count: 1\n"
            "      path: ../web/modules/custom/demo/demo.module\n"
        )
        config["tools"]["phpstan"] = base | {"baseline": "quality/baseline.neon"}
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "phpstan"])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(len(result["findings"]), 0, result)
        source = scenario.root / scenario.spec["changed_files"][0]
        source.write_text(
            "<?php\n/**\n * Old API.\n *\n * @deprecated\n */\n"
            "function demo_old(): void {\n}\ndemo_old();\n"
        )
        for options, count in (({}, 1), ({"deprecations": False}, 0)):
            with test.subTest(options=options):
                config["tools"]["phpstan"] = base | options
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "phpstan"])
                test.assertEqual(len(result["findings"]), count, result)
                test.assertEqual(result["exit_code"], 1 if count else 0, result)


def check_phpstan_project_config(test, configure, run):
    with materialize(FIXTURES / "phpstan-type-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        source = scenario.root / scenario.spec["changed_files"][0]
        neon = scenario.root / "phpstan.neon"
        neon.write_text("parameters:\n  level: 0\n")
        config["tools"]["phpstan"] = {"drupal": False}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, ["--only", "phpstan"])["exit_code"], 0)
        (scenario.root / "phpstan.neon.dist").write_text("parameters:\n  level: 5\n")
        test.assertEqual(run(scenario, ["--only", "phpstan"])["exit_code"], 0)
        for options, count in (({}, 1), ({"level": 0}, 0)):
            with test.subTest(options=options):
                config["tools"]["phpstan"] = {"drupal": False, "discover_config": False} | options
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "phpstan"])
                test.assertEqual(result["exit_code"], 1 if count else 0, result)
                test.assertEqual(len(result["findings"]), count, result)
        neon.write_text("parameters: [\n")
        config["tools"]["phpstan"] = {"drupal": False, "required": True}
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "phpstan"])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"])
        neon.unlink()
        (scenario.root / "phpstan.neon.dist").unlink()
        config["tools"]["phpstan"] = {"drupal": False}
        filename.write_text(json.dumps(config))
        source.write_text("<?php\nfunction demo_value(): int {\n  return demo_vendor_value();\n}\n")
        result = run(scenario, ["--only", "phpstan"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["rule"] for row in result["findings"]], ["function.notFound"])
        (scenario.root / "composer.json").write_text('{"config": {"vendor-dir": "libs"}}\n')
        autoload = scenario.root / "libs/autoload.php"
        autoload.parent.mkdir()
        autoload.write_text("<?php\nfunction demo_vendor_value(): int {\n  return 1;\n}\n")
        result = run(scenario, ["--only", "phpstan"])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(len(result["findings"]), 0, result)
