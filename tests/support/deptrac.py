import json
import shutil

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize


def check_deptrac(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        relative = "web/modules/custom/demo/architecture"
        project = scenario.root / relative
        shutil.copytree(FIXTURES.parent / "deptrac", project)
        filename = project / "src/Domain/Service.php"
        source = filename.read_text()
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS}
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        status = next(row for row in result["checks"] if row["id"] == "deptrac")
        test.assertEqual(status["reason"], "disabled", result)
        result = run(
            scenario, ["--only", "deptrac", filename.relative_to(scenario.root).as_posix()]
        )
        status = next(row for row in result["checks"] if row["id"] == "deptrac")
        test.assertEqual(status["reason"], "no_config", result)
        settings = {"enabled": True, "config": relative + "/deptrac.yaml"}
        config["tools"]["deptrac"] = settings
        config_path.write_text(json.dumps(config))
        for arguments in (
            [],
            ["--mode", "full"],
            ["--whole-files"],
            [filename.relative_to(scenario.root).as_posix()],
        ):
            result = run(scenario, arguments)
            test.assertEqual(result["exit_code"], 1, result)
            test.assertTrue(result["complete"], result)
            test.assertTrue(result["findings"], result)
            for finding in result["findings"]:
                test.assertEqual(finding["rule"], "deptrac.violation", result)
                test.assertEqual(finding["severity"], "error", result)
                test.assertEqual(finding["path"], filename.relative_to(scenario.root).as_posix())
        settings["config"] = ""
        (scenario.root / "deptrac.yaml").write_text(
            (project / "deptrac.yaml")
            .read_text()
            .replace("paths: [src]", f"paths: [{relative}/src]")
        )
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 1, result)
        (scenario.root / "deptrac.yaml").unlink()
        settings.update({"config": relative + "/deptrac.yaml", "fail_on": "never"})
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["findings"], result)
        (project / "deptrac.yaml").write_text(
            "imports:\n  - resource: baseline.yaml\n" + (project / "deptrac.yaml").read_text()
        )
        (project / "baseline.yaml").write_text(
            "deptrac:\n  skip_violations:\n"
            "    'App\\Domain\\Service': ['App\\Infrastructure\\Store']\n"
        )
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(result["findings"], [], result)
        settings.update({"report_skipped": True, "fail_on": "error"})
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["findings"], result)
        test.assertTrue(all(row["severity"] == "info" for row in result["findings"]), result)
        test.assertEqual(filename.read_text(), source)
        (project / "src/Infrastructure/Store.php").write_text("<?php\nclass Store {\n")
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"], result)
        settings["required"] = False
        config_path.write_text(json.dumps(config))
        result = run(scenario, [filename.relative_to(scenario.root).as_posix()])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertFalse(result["complete"], result)
