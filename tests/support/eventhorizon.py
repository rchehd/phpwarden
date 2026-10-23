import json

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize


def check_eventhorizon(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/cache example.module"
        filename.write_text("<?php\nfunction example() {\n  return ['#markup' => 'demo'];\n}\n")
        name = filename.relative_to(scenario.root).as_posix()
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS}
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        status = next(row for row in result["checks"] if row["id"] == "eventhorizon")
        test.assertEqual(status["reason"], "disabled", result)
        for arguments in (
            [],
            ["--mode", "full"],
            ["--whole-files"],
            [filename.relative_to(scenario.root).as_posix()],
        ):
            result = run(scenario, ["--only", "eventhorizon", *arguments])
            test.assertEqual(result["exit_code"], 0, result)
            test.assertTrue(result["complete"], result)
            test.assertTrue(
                any(
                    row["rule"] == "caching.missing_cache_metadata"
                    and row["path"] == name
                    and row["severity"] == "warning"
                    for row in result["findings"]
                ),
                result,
            )
        config["tools"]["eventhorizon"] = {"enabled": True, "fail_on": "warning"}
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 1, result)
        config["tools"]["eventhorizon"]["exclude_rules"] = ["caching.missing_cache_metadata"]
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertFalse(
            any(row["rule"] == "caching.missing_cache_metadata" for row in result["findings"]),
            result,
        )


def check_eventhorizon_config(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        directory = scenario.root / "config/site-one"
        directory.mkdir(parents=True)
        filename = directory / "field.storage.node.field_lonely.yml"
        filename.write_text("field_name: field_lonely\nentity_type: node\ntype: string\n")
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["scope"]["include"].append("config/site-one")
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "eventhorizon": {"enabled": True, "caching": False, "config_sync": ["config/site-one"]}
        }
        config_path.write_text(json.dumps(config))
        for arguments in (
            [],
            ["--mode", "full"],
            ["--whole-files"],
            [filename.relative_to(scenario.root).as_posix()],
        ):
            result = run(scenario, arguments)
            test.assertEqual(result["exit_code"], 0, result)
            test.assertTrue(result["complete"], result)
            test.assertEqual(len(result["findings"]), 1, result)
            finding = result["findings"][0]
            test.assertEqual(finding["rule"], "config.unused_field", result)
            test.assertEqual(finding["kind"], "global", result)
            test.assertIsNone(finding["path"], result)
        (directory / "broken.yml").write_text("broken: [\n")
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"], result)
        test.assertEqual(result["findings"], [], result)
