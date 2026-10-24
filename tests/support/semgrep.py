import json

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize


def check_semgrep(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/request handler.module"
        filename.write_text("<?php\n$value = $_GET['cmd'];\nsystem($value);\n")
        safe = filename.with_name("safe.module")
        safe.write_text("<?php\necho htmlspecialchars($_GET['html'], ENT_QUOTES, 'UTF-8');\n")
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "semgrep": {"enabled": False, "fail_on": "never"}
        }
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        status = next(row for row in result["checks"] if row["id"] == "semgrep")
        test.assertEqual(status["reason"], "disabled", result)
        for arguments in (
            [],
            ["--mode", "full"],
            ["--whole-files"],
            [filename.relative_to(scenario.root).as_posix()],
        ):
            result = run(scenario, ["--only", "semgrep", *arguments])
            test.assertEqual(result["exit_code"], 0, result)
            test.assertTrue(result["complete"], result)
            test.assertEqual(len(result["findings"]), 1, result)
            finding = result["findings"][0]
            test.assertEqual(finding["rule"], "phpwarden.php.request-command-injection", result)
            test.assertEqual(
                finding["path"], filename.relative_to(scenario.root).as_posix(), result
            )
            test.assertEqual(len(finding["fingerprint"]), 64, result)
        config["tools"]["semgrep"] = {}
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(len(result["findings"]), 1, result)
        baseline = scenario.root / "semgrep-baseline.json"
        baseline.write_text(
            json.dumps(
                {"version": 1, "fingerprints": [row["fingerprint"] for row in result["findings"]]}
            )
        )
        config["tools"]["semgrep"]["baseline"] = baseline.name
        config_path.write_text(json.dumps(config))
        filename.write_text("<?php\n\n$value = $_GET['cmd'];\nsystem($value);\n")
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(result["findings"], [], result)
        filename.write_text("<?php\nfunction broken( {\n")
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"], result)
        config["tools"]["semgrep"]["required"] = False
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertFalse(result["complete"], result)


def check_semgrep_project_rules(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/custom-rule.module"
        filename.write_text("<?php\necho 42;\n")
        (scenario.root / ".semgrep.yml").write_text(
            "rules:\n  - id: custom.echo\n    languages: [php]\n"
            "    severity: INFO\n    message: Review this echo\n    pattern: echo 42;\n"
        )
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "semgrep": {"enabled": True, "use_defaults": False}
        }
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(len(result["findings"]), 1, result)
        test.assertEqual(result["findings"][0]["rule"], "custom.echo", result)
        config["tools"]["semgrep"]["exclude_rules"] = ["custom.echo"]
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        status = next(row for row in result["checks"] if row["id"] == "semgrep")
        test.assertEqual(status["reason"], "no_rules", result)
