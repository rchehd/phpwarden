import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "twig-cs-fixer"
TOOLS = (
    "composer-audit",
    "composer-validate",
    "debug",
    "drupal-config",
    "phpcs",
    "phpmd",
    "phpstan",
    "syntax",
    "twig-cs-fixer",
    "whitespace",
    "yaml",
)
TEMPLATES = "web/modules/custom/demo/templates"
CONFIG_HEAD = "<?php\n\n$ruleset = new TwigCsFixer\\Ruleset\\Ruleset();\n"
CONFIG_TAIL = (
    "$config = new TwigCsFixer\\Config\\Config();\n"
    "$config->setRuleset($ruleset);\n\n"
    "return $config;\n"
)


def sort_key(row):
    return (row[0], row[1] or "", row[2] or 0)


def write_config(path, rules=""):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(CONFIG_HEAD + rules + CONFIG_TAIL)


def check_twig_cs_fixer_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                filename = scenario.root / ".phpwarden.yaml"
                config = json.loads(filename.read_text())
                config.setdefault("tools", {})["twig-cs-fixer"] = scenario.spec["settings"] | {
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
                test.assertEqual([row["id"] for row in result["checks"]], list(TOOLS))
                for row in result["findings"]:
                    if row["line"] is None:
                        test.assertEqual(row["kind"], "global", row)
                        test.assertEqual(row["rule"], "File.Invalid", row)
                        test.assertTrue(
                            any(
                                row["message"].startswith(f"Twig template {json.dumps(name)}: ")
                                for name in scenario.spec["changed_files"]
                            ),
                            row,
                        )
                        continue
                    test.assertEqual(row["kind"], "code", row)
                    test.assertIn(row["severity"], ("error", "warning", "info"), row)
                    test.assertTrue(row["rule"] and "." in row["rule"], row)
                test.assertEqual(list(scenario.root.rglob("executed")), [])


def check_twig_cs_fixer_settings(test, configure, run):
    with materialize(FIXTURES / "twig-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {name: {"enabled": False} for name in TOOLS}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        test.assertEqual(run(scenario, ["--only", "twig-cs-fixer"])["exit_code"], 1)
        for options, count, exit_code in (
            ({"fail_on": "never"}, 2, 0),
            ({"exclude": ["**/*.twig"]}, 0, 0),
            ({"include": ["web/modules/custom/missing"]}, 0, 0),
        ):
            with test.subTest(options=options):
                config["tools"]["twig-cs-fixer"] = options
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "twig-cs-fixer"])
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual(len(result["findings"]), count, result)
        extra = scenario.root / TEMPLATES / "extra.tpl"
        extra.write_text("<p>{{title}}</p>\n")
        for options, count in (({}, 2), ({"extra_extensions": [".tpl"]}, 4)):
            with test.subTest(options=options):
                config["tools"]["twig-cs-fixer"] = options
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "twig-cs-fixer"])
                test.assertEqual(len(result["findings"]), count, result)
        extra.unlink()
        for required in (True, False):
            config["tools"]["twig-cs-fixer"] = {
                "config": "missing.php",
                "required": required,
                "fail_on": "never",
            }
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "twig-cs-fixer"])
            test.assertEqual(result["exit_code"], 3 if required else 0, result)
            test.assertFalse(result["complete"])
        bad = scenario.root / "quality/bad.php"
        bad.parent.mkdir()
        bad.write_text("<?php\n\nreturn [];\n")
        config["tools"]["twig-cs-fixer"] = {"config": "quality/bad.php"}
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "twig-cs-fixer"])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"])


def check_twig_cs_fixer_project_config(test, configure, run):
    with materialize(FIXTURES / "twig-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"]["twig-cs-fixer"] = {}
        filename.write_text(json.dumps(config))
        only = ["--only", "twig-cs-fixer"]
        dist = scenario.root / ".twig-cs-fixer.dist.php"
        write_config(dist)
        test.assertEqual(run(scenario, only)["exit_code"], 0)
        primary = scenario.root / ".twig-cs-fixer.php"
        write_config(primary, "$ruleset->addStandard(new TwigCsFixer\\Standard\\TwigCsFixer());\n")
        test.assertEqual(run(scenario, only)["exit_code"], 1)
        config["tools"]["twig-cs-fixer"] = {"discover_config": False}
        filename.write_text(json.dumps(config))
        result = run(scenario, only)
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual(len(result["findings"]), 2, result)
        primary.unlink()
        result = run(scenario, only)
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual(len(result["findings"]), 2, result)
        config["tools"]["twig-cs-fixer"] = {"config": ".twig-cs-fixer.dist.php"}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, only)["exit_code"], 0)
        dist.unlink()
        autoload = scenario.root / "vendor/autoload.php"
        autoload.parent.mkdir()
        autoload.write_text("<?php file_put_contents(__DIR__ . '/../executed', 'bad');\n")
        rules = scenario.root / "quality/rules.php"
        rules.parent.mkdir()
        rules.write_text(
            "<?php\n\nreturn ['TwigCsFixer\\\\Rules\\\\Delimiter\\\\DelimiterSpacingRule'];\n"
        )
        primary.write_text(
            "<?php\n\n$ruleset = new TwigCsFixer\\Ruleset\\Ruleset();\n"
            "foreach (require __DIR__ . '/quality/rules.php' as $rule) {\n"
            "    $ruleset->addRule(new $rule());\n"
            "}\n" + CONFIG_TAIL
        )
        config["tools"]["twig-cs-fixer"] = {"extra_inputs": ["quality"]}
        filename.write_text(json.dumps(config))
        result = run(scenario, only)
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual(len(result["findings"]), 2, result)
        test.assertEqual(list(scenario.root.rglob("executed")), [])
