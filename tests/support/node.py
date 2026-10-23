import json

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize


def check_node_tools(test, configure, run):
    for checker in ("eslint", "stylelint", "jscpd"):
        with test.subTest(checker=checker), materialize(FIXTURES / "phpmd-added") as scenario:
            configure(scenario)
            directory = scenario.root / "web/modules/custom/demo"
            if checker == "eslint":
                (directory / ".eslintrc.cjs").write_text(
                    "module.exports = { root: true, parserOptions: { ecmaVersion: 2022 }, "
                    "rules: { 'no-unused-vars': 'error' } };\n"
                )
                filename = directory / "new file.js"
                filename.write_text("const unused = 1;\n")
                settings = {}
            elif checker == "stylelint":
                (directory / ".stylelintrc.json").write_text(
                    json.dumps({"rules": {"block-no-empty": True}})
                )
                filename = directory / "new file.css"
                filename.write_text("a {}\n")
                settings = {}
            else:
                filename = directory / "new [copy].module"
                content = (
                    "<?php\nfunction duplicated(): void {\n  $first = 1;\n"
                    "  $second = 2;\n  echo $first + $second;\n}\n"
                )
                filename.write_text(content)
                (directory / "original.module").write_text(content)
                settings = {"min_tokens": 10, "min_lines": 3, "fail_on": "warning"}
            config_path = scenario.root / ".phpwarden.yaml"
            config = json.loads(config_path.read_text())
            config["tools"] = {name: {"enabled": False} for name in TOOLS} | {
                checker: {"enabled": True} | settings
            }
            config_path.write_text(json.dumps(config))
            for arguments in (
                [],
                ["--mode", "full"],
                ["--whole-files"],
                [filename.relative_to(scenario.root).as_posix()],
            ):
                with test.subTest(arguments=arguments):
                    result = run(scenario, arguments)
                    test.assertEqual(result["exit_code"], 1, result)
                    test.assertTrue(result["complete"], result)
                    test.assertTrue(result["findings"], result)
                    test.assertTrue(
                        all(row["checker"] == checker for row in result["findings"]), result
                    )
                    if arguments == [filename.relative_to(scenario.root).as_posix()]:
                        test.assertTrue(
                            all(
                                row["path"] == filename.relative_to(scenario.root).as_posix()
                                for row in result["findings"]
                            ),
                            result,
                        )


def check_node_no_config(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        filename = scenario.root / "web/modules/custom/demo/new.js"
        filename.write_text("const unused = 1;\n")
        result = run(scenario, ["--only", "eslint", "--mode", "full"])
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(result["checks"][0]["status"], "skipped", result)
        test.assertEqual(result["checks"][0]["reason"], "no_config", result)
        test.assertEqual(result["findings"], [], result)
