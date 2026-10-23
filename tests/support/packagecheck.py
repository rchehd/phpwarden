import json

from tests.support.phpmd import FIXTURES, TOOLS
from tests.support.scenarios import materialize


def check_package_tools(test, configure, run):
    with materialize(FIXTURES / "phpmd-added") as scenario:
        configure(scenario)
        package = scenario.root / "package"
        package.mkdir()
        manifest = {
            "name": "demo/root",
            "version": "1.0.0",
            "require": {"php": ">=8.1", "acme/a": "^1", "acme/b": "^1"},
            "autoload": {"psr-4": {"Demo\\": "src/"}},
        }
        (package / "composer.json").write_text(json.dumps(manifest))
        source = package / "src/Demo.php"
        source.parent.mkdir()
        source.write_text(
            "<?php\nnamespace Demo;\n"
            "class Demo { public function go(): void { new \\Acme\\A\\Used(); } }\n"
        )
        packages = []
        for name, symbol in (("a", "Used"), ("b", "Unused")):
            dependency = package / "vendor/acme" / name
            (dependency / "src").mkdir(parents=True)
            meta = {
                "name": "acme/" + name,
                "version": "1.0.0",
                "type": "library",
                "require": {"php": ">=8.1"},
                "autoload": {"psr-4": {"Acme\\" + name.upper() + "\\": "src/"}},
            }
            (dependency / "composer.json").write_text(json.dumps(meta))
            (dependency / "src" / (symbol + ".php")).write_text(
                "<?php\nnamespace Acme\\" + name.upper() + ";\nclass " + symbol + " {}\n"
            )
            packages.append(
                {
                    **meta,
                    "install-path": "../acme/" + name,
                    "source": {
                        "type": "git",
                        "url": "https://example.com/acme/" + name,
                        "reference": "fixture",
                    },
                }
            )
        metadata = package / "vendor/composer"
        metadata.mkdir()
        (metadata / "installed.json").write_text(
            json.dumps({"packages": packages, "dev": True, "dev-package-names": []})
        )
        versions = ",".join(
            "'acme/" + name + "'=>['pretty_version'=>'1.0.0','version'=>'1.0.0.0',"
            "'type'=>'library','install_path'=>__DIR__.'/../acme/"
            + name
            + "','aliases'=>[],'dev_requirement'=>false]"
            for name in ("a", "b")
        )
        (metadata / "installed.php").write_text(
            "<?php return ['root'=>['name'=>'demo/root','version'=>'1.0.0.0',"
            "'pretty_version'=>'1.0.0','type'=>'library','install_path'=>__DIR__.'/../..',"
            "'aliases'=>[],'dev'=>true], 'versions'=>[" + versions + "]];"
        )
        config_path = scenario.root / ".phpwarden.yaml"
        config = json.loads(config_path.read_text())
        config["tools"] = {tool: {"enabled": False} for tool in TOOLS}
        for tool in ("composer-require-checker", "composer-unused"):
            config["tools"][tool] = {"enabled": True, "composer_root": "package"}
        config_path.write_text(json.dumps(config))
        only = ["--only", "composer-require-checker,composer-unused"]
        result = run(scenario, only)
        test.assertTrue(
            all(row["reason"] == "full_scan_required" for row in result["checks"]), result
        )
        args = [*only, "--mode", "full"]
        result = run(scenario, args)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(
            [row["rule"] for row in result["findings"]], ["composer-unused.candidate"], result
        )
        source.write_text("<?php\nnamespace Demo;\nclass Demo extends \\Undeclared\\Missing {}\n")
        config["tools"]["composer-require-checker"]["fail_on"] = "warning"
        config_path.write_text(json.dumps(config))
        result = run(scenario, args)
        test.assertEqual(result["exit_code"], 1, result)
        test.assertTrue(
            any(
                row["rule"] == "composer-require-checker.unknown_symbol"
                for row in result["findings"]
            ),
            result,
        )
        config["tools"]["composer-require-checker"]["allowed_symbols"] = ["Undeclared\\Missing"]
        config["tools"]["composer-unused"]["exclude_packages"] = ["acme/a", "acme/b"]
        config_path.write_text(json.dumps(config))
        result = run(scenario, args)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertEqual(result["findings"], [], result)
        packages[0]["install-path"] = "../../web/core"
        (metadata / "installed.json").write_text(json.dumps({"packages": packages}))
        result = run(scenario, args)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(
            all(row["reason"] == "unsupported_layout" for row in result["checks"]), result
        )
        (metadata / "installed.json").unlink()
        result = run(scenario, args)
        test.assertEqual(result["exit_code"], 3, result)
        test.assertFalse(result["complete"], result)
