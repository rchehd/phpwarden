import json
from pathlib import Path

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "phpcs"


def check_phpcs_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                filename = scenario.root / ".phpwarden.yaml"
                config = json.loads(filename.read_text())
                config.setdefault("tools", {})["phpcs"] = scenario.spec["settings"] | {
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
                test.assertEqual(
                    [row["id"] for row in result["checks"]],
                    [
                        "debug",
                        "drupal-config",
                        "phpcs",
                        "phpstan",
                        "syntax",
                        "twig-cs-fixer",
                        "whitespace",
                        "yaml",
                    ],
                )
                for row in result["findings"]:
                    test.assertEqual(row["kind"], "code")
                    test.assertTrue(
                        row["rule"].startswith(("Drupal.", "Generic.", "PHPCompatibility.")), row
                    )


def check_phpcs_settings(test, configure, run):
    with materialize(FIXTURES / "phpcs-drupal-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        config["tools"] = {
            name: {"enabled": False}
            for name in (
                "debug",
                "drupal-config",
                "phpcs",
                "phpstan",
                "syntax",
                "twig-cs-fixer",
                "whitespace",
                "yaml",
            )
        }
        base = {"sniffs": ["Drupal.ControlStructures.ControlSignature"]}
        config["tools"]["phpcs"] = base | {"enabled": False}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        test.assertEqual(run(scenario, ["--only", "phpcs"])["exit_code"], 1)
        config["tools"]["phpcs"] = {}
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "phpcs"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(result["diagnostics"], [])
        for options, count, exit_code in (
            ({"fail_on": "never"}, 1, 0),
            ({"error_severity": 0}, 0, 0),
            (
                {
                    "sniffs": [
                        "Drupal.ControlStructures.ControlSignature",
                        "Generic.PHP.DisallowShortOpenTag",
                    ],
                    "exclude_sniffs": ["Drupal.ControlStructures.ControlSignature"],
                },
                0,
                0,
            ),
            ({"exclude": ["**/*.module"]}, 0, 0),
            ({"include": ["web/modules/custom/missing"]}, 0, 0),
        ):
            with test.subTest(options=options):
                config["tools"]["phpcs"] = base | options
                filename.write_text(json.dumps(config))
                result = run(scenario, ["--only", "phpcs"])
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual(len(result["findings"]), count, result)
        for required in (True, False):
            config["tools"]["phpcs"] = {
                "config": "missing.xml",
                "required": required,
                "fail_on": "never",
            }
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "phpcs"])
            test.assertEqual(result["exit_code"], 3 if required else 0, result)
            test.assertFalse(result["complete"])
        source = scenario.root / scenario.spec["changed_files"][0]
        source.write_text("<?php\narray_find([], fn ($value) => true);\n")
        for version, count in (("", 0), ("8.3", 1)):
            config["tools"]["phpcs"] = {
                "standards": ["PHPCompatibility"],
                "compatibility": {"enabled": True, "test_version": version},
            }
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "phpcs"])
            test.assertEqual(len(result["findings"]), count, result)
            test.assertEqual(result["exit_code"], 1 if count else 0, result)


def check_phpcs_project_rules(test, configure, run):
    with materialize(FIXTURES / "phpcs-drupal-added") as scenario:
        configure(scenario)
        filename = scenario.root / ".phpwarden.yaml"
        config = json.loads(filename.read_text())
        source = scenario.root / scenario.spec["changed_files"][0]
        root_rules = scenario.root / "phpcs.xml"
        root_rules.write_text(
            '<ruleset><rule ref="Generic.WhiteSpace.DisallowTabIndent"/></ruleset>\n'
        )
        source.write_text("<?php\nif(true) {\n  echo 1;\n}\n")
        config["tools"]["phpcs"] = {"enabled": True}
        filename.write_text(json.dumps(config))
        test.assertEqual(run(scenario, ["--only", "phpcs"])["exit_code"], 0)
        root_rules.write_text(
            '<ruleset><rule ref="Generic.WhiteSpace.DisallowTabIndent">'
            "<type>warning</type></rule></ruleset>\n"
        )
        source.write_text("<?php\nif (true) {\n\techo 1;\n}\n")
        for policy, exit_code in (("error", 0), ("warning", 1), ("never", 0)):
            config["tools"]["phpcs"]["fail_on"] = policy
            filename.write_text(json.dumps(config))
            result = run(scenario, ["--only", "phpcs"])
            test.assertEqual(result["exit_code"], exit_code, result)
            test.assertEqual([row["severity"] for row in result["findings"]], ["warning"])
        root_rules.write_text('<ruleset><rule ref="NoSuchStandard"/></ruleset>\n')
        config["tools"]["phpcs"] = {"required": True, "fail_on": "never"}
        filename.write_text(json.dumps(config))
        result = run(scenario, ["--only", "phpcs"])
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"])
        standard = scenario.root / "quality/Acme"
        sniff = standard / "Sniffs/Debug/BanEchoSniff.php"
        sniff.parent.mkdir(parents=True)
        (standard / "fixture.xml").write_text("invalid XML fixture <")
        (standard / "ruleset.xml").write_text(
            '<ruleset name="Acme"><rule ref="Acme.Debug.BanEcho"/></ruleset>\n'
        )
        sniff.write_text(
            "<?php\nnamespace Acme\\Sniffs\\Debug;\n"
            "class BanEchoSniff implements \\PHP_CodeSniffer\\Sniffs\\Sniff {\n"
            "  public function register() { return [T_ECHO]; }\n"
            "  public function process(\\PHP_CodeSniffer\\Files\\File $file, $ptr) {\n"
            "    $file->addError('Echo is forbidden', $ptr, 'Found');\n"
            "  }\n}\n"
        )
        root_rules.write_text('<ruleset><rule ref="Acme"/></ruleset>\n')
        config["tools"]["phpcs"] = {"standard_paths": ["quality/Acme"]}
        filename.write_text(json.dumps(config))
        vendor = scenario.root / "vendor"
        vendor.mkdir()
        (vendor / "autoload.php").write_text(
            "<?php file_put_contents(__DIR__ . '/../executed', 'bad');\n"
        )
        result = run(scenario, ["--only", "phpcs"])
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["rule"] for row in result["findings"]], ["Acme.Debug.BanEcho.Found"])
        test.assertFalse((scenario.root / "executed").exists())
