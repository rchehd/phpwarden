import json

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize


def check_fix(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        directory = scenario.root / "web/modules/custom/demo"
        php = directory / "fix.php"
        twig = directory / "fix.twig"
        php.write_text("<?php\nif (true) {\n\techo 1;\n}\n")
        twig.write_text("<p>{{title}}</p>\n")
        unselected = directory / "unselected.php"
        unselected.write_text("<?php\nif (true) {\n\techo 2;\n}\n")
        before = unselected.read_bytes()
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS} | {
            "phpcs": {
                "enabled": True,
                "standards": ["PSR12"],
                "sniffs": ["Generic.WhiteSpace.DisallowTabIndent"],
            },
            "twig-cs-fixer": {"enabled": True},
        }
        config_path.write_text(json.dumps(config))
        args = ["--fix", "--only", "phpcs,twig-cs-fixer"]
        paths = [
            php.relative_to(scenario.root).as_posix(),
            twig.relative_to(scenario.root).as_posix(),
        ]
        result = run(scenario, args + paths)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(sorted(result["fixed_files"]), sorted(paths), result)
        test.assertNotIn("\t", php.read_text())
        test.assertEqual(twig.read_text(), "<p>{{ title }}</p>\n")
        test.assertEqual(unselected.read_bytes(), before)
        result = run(scenario, args + paths)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(result.get("fixed_files", []), [], result)
        result = run(scenario, ["--fix", "--only", "syntax", *paths])
        test.assertEqual(result["exit_code"], 2, result)
