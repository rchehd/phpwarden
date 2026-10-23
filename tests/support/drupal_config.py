import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "drupal-config"


def check_drupal_config_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                filename = scenario.root / ".phpwarden.yaml"
                config = json.loads(filename.read_text())
                config.setdefault("tools", {})["drupal-config"] = scenario.spec.get("settings", {})
                filename.write_text(json.dumps(config))
                arguments = {
                    "diff": [],
                    "full": ["--mode", "full"],
                    "whole-files": ["--whole-files"],
                    "paths": scenario.spec["changed_files"],
                }[mode]
                result = run(scenario, arguments)
                expected = sorted(
                    (row["checker"], row["path"], row["line"]) for row in scenario.spec["findings"]
                )
                actual = sorted(
                    (row["checker"], row["path"], row["line"]) for row in result["findings"]
                )
                test.assertEqual(actual, expected, result)
                test.assertEqual(result["exit_code"], 1 if expected else 0, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                test.assertEqual(
                    [row["id"] for row in result["checks"]],
                    [
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
                    ],
                )
                for row in result["findings"]:
                    test.assertEqual(row["kind"], "code")
                    test.assertEqual(row["rule"], "drupal.config.forbidden_key")
                if mode == "diff" and directory.name == "drupal-config-deletion-only":
                    test.assertEqual(
                        result["scope"]["line_map"][scenario.spec["changed_files"][0]], []
                    )


def check_drupal_config_settings(test, configure, run):
    with materialize(FIXTURES / "drupal-config-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {
            name: {"enabled": False}
            for name in (
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
        }
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        result = run(scenario, ["--only", "drupal-config"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["id"] for row in result["checks"]], ["drupal-config"])
        test.assertEqual(len(result["findings"]), 2)
        for settings, count, exit_code in (
            ({"fail_on": "never"}, 2, 0),
            ({"forbidden_keys": ["uuid"]}, 1, 1),
            ({"forbidden_keys": ["status"]}, 1, 1),
            ({"forbidden_keys": []}, 0, 0),
            ({"directories": ["config/optional"]}, 0, 0),
            ({"exclude": ["**/*.yml"]}, 0, 0),
            ({"include": ["web/modules/custom/missing"]}, 0, 0),
        ):
            with test.subTest(settings=settings):
                config["tools"]["drupal-config"] = settings
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "drupal-config"])
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual(len(result["findings"]), count, result)
                test.assertTrue(result["complete"], result)
        source = scenario.root / scenario.spec["changed_files"][0]
        source.write_text("uuid: [broken\n")
        for required in (True, False):
            config["tools"]["drupal-config"] = {"required": required, "fail_on": "never"}
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "drupal-config"])
            test.assertEqual(result["exit_code"], 3 if required else 0, result)
            test.assertFalse(result["complete"], result)
            test.assertEqual(result["diagnostics"][0]["code"], "yaml_invalid")
        config["tools"]["drupal-config"] = {}
        filename.write_text(json.dumps(config))
        for data in ("status: true\n!label uuid: value\n", "base: &name uuid\n? *name\n: value\n"):
            with test.subTest(key_syntax=data):
                source.write_text(data)
                result = run(scenario, ["--only", "drupal-config"])
                test.assertEqual(result["exit_code"], 1, result)
                test.assertEqual([row["line"] for row in result["findings"]], [2])
                test.assertIn('"uuid"', result["findings"][0]["message"])


def check_drupal_config_custom_layout(test, configure, run):
    with materialize(FIXTURES / "drupal-config-clean") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["scope"]["include"] = ["."]
        custom = scenario.root / "packages/demo/defaults/demo.yml"
        custom.parent.mkdir(parents=True)
        custom.write_text("uuid: custom\n")
        site = scenario.root / "config/install/site.yml"
        site.parent.mkdir(parents=True)
        site.write_text("uuid: site-export\n_core: {}\n")
        config.setdefault("tools", {})["drupal-config"] = {"directories": ["defaults"]}
        filename.write_text(json.dumps(config))
        test.assertEqual(
            run(scenario, ["--only", "drupal-config", "--mode", "full"])["exit_code"], 0
        )
        config["tools"]["drupal-config"]["module_roots"] = ["packages/demo"]
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "drupal-config", "--mode", "full"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual(
            [row["path"] for row in result["findings"]], ["packages/demo/defaults/demo.yml"]
        )
        custom.write_text("status: true\n")
        source = scenario.root / "config/optional/standalone.yml"
        source.parent.mkdir(parents=True)
        source.write_text("uuid: standalone-module\n")
        config["tools"]["drupal-config"] = {
            "module_roots": ["."],
            "directories": ["config/optional"],
        }
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "drupal-config", "--mode", "full"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual(
            [row["path"] for row in result["findings"]], ["config/optional/standalone.yml"]
        )
        vendor = scenario.root / "vendor"
        vendor.mkdir()
        (vendor / "autoload.php").write_text(
            "<?php file_put_contents(__DIR__ . '/../executed', 'bad');\n"
        )
        test.assertEqual(
            run(scenario, ["--only", "drupal-config", "--mode", "full"])["exit_code"], 1
        )
        test.assertFalse((scenario.root / "executed").exists())
