import json

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize

MESSAGE_RULE = r"DrupalRector\Drupal8\Rector\Deprecation\DrupalSetMessageRector"


def check_rector_profile_preset(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/upgrade.module"
        source = "<?php\ndrupal_set_message('hello');\n"
        filename.write_text(source)
        composer = scenario.root / "platform"
        metadata = composer / "dependencies/composer/installed.json"
        metadata.parent.mkdir(parents=True)
        package = {"name": "drupal/core", "version": "11.4.10", "version_normalized": "11.4.10.0"}
        (composer / "composer.json").write_text(
            json.dumps({"config": {"vendor-dir": "dependencies"}})
        )
        (composer / "composer.lock").write_text(json.dumps({"packages": [package]}))
        metadata.write_text(json.dumps({"packages": [package]}))
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["composer_root"] = "platform"
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "rector": {"enabled": True}
        }
        config_path.write_text(json.dumps(config))
        arguments = [filename.relative_to(scenario.root).as_posix()]
        result = run(scenario, arguments)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(result["environment"]["framework_version"], "11.4.10", result)
        test.assertEqual(len(result["findings"]), 1, result)
        test.assertIn("DrupalSetMessageRector", result["findings"][0]["message"])
        test.assertEqual(filename.read_text(), source)

        metadata.unlink()
        result = run(scenario, arguments)
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"], result)
        test.assertEqual(result["diagnostics"][0]["code"], "dependencies_missing", result)
        test.assertIn("composer install", result["diagnostics"][0]["message"])

        config["tools"]["rector"]["rules"] = [MESSAGE_RULE]
        config_path.write_text(json.dumps(config))
        result = run(scenario, arguments)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(len(result["findings"]), 1, result)
        test.assertEqual(filename.read_text(), source)


def check_rector(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/upgrade demo.module"
        source = "<?php\ndrupal_set_message('hello');\n"
        filename.write_text(source)
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "rector": {"enabled": False, "rules": [MESSAGE_RULE]}
        }
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        status = next(row for row in result["checks"] if row["id"] == "rector")
        test.assertEqual(status["reason"], "disabled", result)
        for arguments in (
            [],
            ["--mode", "full"],
            ["--whole-files"],
            [filename.relative_to(scenario.root).as_posix()],
        ):
            result = run(scenario, ["--only", "rector", *arguments])
            test.assertEqual(result["exit_code"], 0, result)
            test.assertTrue(result["complete"], result)
            test.assertEqual(len(result["findings"]), 1, result)
            finding = result["findings"][0]
            test.assertEqual(finding["checker"], "rector", result)
            test.assertEqual(finding["rule"], "rector.upgrade", result)
            test.assertEqual(finding["line"], 2, result)
            test.assertEqual(finding["severity"], "warning", result)
            test.assertEqual(filename.read_text(), source)
        config["tools"]["rector"]["enabled"] = True
        config["tools"]["rector"]["fail_on"] = "warning"
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 1, result)
        filename.write_text("<?php\nfunction broken( {\n")
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"], result)
        config["tools"]["rector"]["required"] = False
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertFalse(result["complete"], result)


def check_rector_project_config(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/custom-rule.module"
        source = "<?php\nold_helper();\n"
        filename.write_text(source)
        (scenario.root / "rector.dist.php").write_text(
            "<?php\nuse Rector\\Config\\RectorConfig;\n"
            "use Rector\\Renaming\\Rector\\FuncCall\\RenameFunctionRector;\n"
            "return static function (RectorConfig $config): void {\n"
            "    $config->ruleWithConfiguration(RenameFunctionRector::class, "
            "['old_helper' => 'new_helper']);\n};\n"
        )
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "rector": {"enabled": True}
        }
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(len(result["findings"]), 1, result)
        test.assertIn("RenameFunctionRector", result["findings"][0]["message"])
        test.assertEqual(filename.read_text(), source)
        config["tools"]["rector"]["skip_rules"] = [
            r"Rector\Renaming\Rector\FuncCall\RenameFunctionRector"
        ]
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(result["findings"], [], result)
