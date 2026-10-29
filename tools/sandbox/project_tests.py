import json
import os
import shutil
import xml.etree.ElementTree as ET

from tools.sandbox.assertions import tree_snapshot


def prepare_unit_project(sandbox):
    sandbox.stage("checking PHPUnit on a private Drupal project with locked dev dependencies")
    root = sandbox.root / "unit-project"
    shutil.copytree(
        sandbox.project,
        root,
        symlinks=True,
        ignore=shutil.ignore_patterns(
            ".git", ".ddev", ".agents", ".codex", ".cursor", ".claude", "node_modules"
        ),
    )
    fixture = sandbox.repository / f"tools/sandbox/drupal{sandbox.major}-tests"
    for name in ("composer.json", "composer.lock"):
        shutil.copyfile(fixture / name, root / name)
    sandbox.run(
        [
            "docker",
            "run",
            "--rm",
            "--name",
            sandbox.name + "-install",
            "--user",
            f"{os.getuid()}:{os.getgid()}",
            "--mount",
            f"type=bind,src={root},dst=/project",
            "--workdir",
            "/project",
            "--env",
            "COMPOSER_HOME=/tmp/composer-home",
            "--entrypoint",
            "php",
            sandbox.image,
            "/usr/local/lib/phpwarden/composer/vendor/bin/composer",
            "install",
            "--no-scripts",
            "--no-interaction",
            "--prefer-dist",
            "--no-progress",
        ],
        timeout=600,
    )
    unit = root / "web/modules/custom/phpwarden_probe/tests/src/Unit"
    unit.mkdir(parents=True, exist_ok=True)
    test = unit / "ProbeTest.php"
    shutil.copyfile(sandbox.repository / "tools/sandbox/unit-fixture/ProbeTest.php", test)
    for name in ("SplitATest.php", "SplitBTest.php"):
        shutil.copyfile(sandbox.repository / "tools/sandbox/unit-fixture" / name, unit / name)
    shutil.copyfile(
        sandbox.repository / "tools/sandbox/unit-fixture/CoverageProbe.php",
        root / "web/modules/custom/phpwarden_probe/src/CoverageProbe.php",
    )
    shutil.copyfile(
        sandbox.repository / "tools/sandbox/unit-fixture/UncoveredProbe.php",
        root / "web/modules/custom/phpwarden_probe/src/UncoveredProbe.php",
    )
    return root, test


def project_tests(sandbox):
    root, test = prepare_unit_project(sandbox)
    original = test.read_bytes()
    before = tree_snapshot(root)
    baseline = sandbox.git_status()
    results = {}

    def run(name, *arguments, expected=0):
        output = sandbox.evidence.log_directory() / name
        result = json.loads(
            sandbox.run(
                [
                    sandbox.binary,
                    "test",
                    "--project",
                    root,
                    "--format=json",
                    "--output",
                    output,
                    *arguments,
                ],
                expected=(expected,),
                timeout=600,
            )
        )
        assert result["exit_code"] == expected
        if result.get("artifacts"):
            assert json.loads((output / "report.json").read_text()) == result
        results[name] = {
            "exit_code": expected,
            "counts": result["counts"],
            "complete": result["complete"],
        }
        return result

    result = run("passed")
    assert result["complete"] and result["counts"]["tests"] == 4
    assert result["phpunit_version"].startswith("9." if sandbox.major == "10" else "11.")
    assert not (root / "web/phpwarden-unit-probe").exists()
    filtered = run("filtered", "--filter=testHotspot")
    assert filtered["complete"] and filtered["counts"]["tests"] == 1
    run("empty-filter", "--filter=phpwardenMissingTest", expected=3)
    coverage = run("coverage", "--coverage")
    metrics = coverage["coverage"]
    assert coverage["complete"] and 0 < metrics["line_percent"] < 100
    assert metrics["driver"] == "pcov" and metrics["max_crap"] > 0
    uncovered = next(method for method in metrics["methods"] if method["name"] == "neverCalled")
    assert uncovered["line_percent"] == 0 and uncovered["crap"] > uncovered["complexity"]
    abstract = next(method for method in metrics["methods"] if method["name"] == "contract")
    assert abstract["complexity"] == 0
    assert all("/tests/" not in method["file"] for method in metrics["methods"])
    union = run(
        "coverage-union",
        "--coverage",
        "web/modules/custom/phpwarden_probe/tests/src/Unit/SplitATest.php",
        "web/modules/custom/phpwarden_probe/tests/src/Unit/SplitBTest.php",
    )
    choose = next(method for method in union["coverage"]["methods"] if method["name"] == "choose")
    assert union["counts"]["tests"] == 2 and choose["line_percent"] == 100
    assert choose["crap"] == choose["complexity"] == 2
    output = sandbox.evidence.log_directory() / "coverage-union/coverage"
    assert (output / "html/index.html").is_file() and (output / "xml/index.xml").is_file()
    assert (output / "clover.xml").is_file() and (output / "crap4j.xml").is_file()
    native = root / "web/core/phpunit-coverage-native.xml"
    empty_source = root / "web/modules/custom/phpwarden_probe/src/EmptyCoverage.php"
    try:
        document = ET.parse(root / "web/core/phpunit.xml.dist")
        phpunit = document.getroot()
        for node in list(phpunit):
            if node.tag in ("testsuites", "testsuite", "coverage", "source"):
                phpunit.remove(node)
        suite = ET.SubElement(
            ET.SubElement(phpunit, "testsuites"), "testsuite", name="coverage-fixture"
        )
        for name in ("SplitATest.php", "SplitBTest.php"):
            ET.SubElement(suite, "file").text = (
                "/workspace/web/modules/custom/phpwarden_probe/tests/src/Unit/" + name
            )
        source = ET.SubElement(phpunit, "coverage" if sandbox.major == "10" else "source")
        included = ET.SubElement(ET.SubElement(source, "include"), "file")
        included.text = "/workspace/web/modules/custom/phpwarden_probe/src/CoverageProbe.php"
        document.write(native, encoding="unicode")
        measured = run(
            "coverage-native",
            "--coverage",
            "--testsuite=coverage-fixture",
            "--phpunit-config=web/core/phpunit-coverage-native.xml",
        )
        assert measured["counts"]["tests"] == 2 and measured["coverage"]["line_percent"] == 100
        assert len(measured["coverage"]["methods"]) == 1
        empty_source.write_text(
            "<?php namespace Drupal\\phpwarden_probe; final class EmptyCoverage {}\n"
        )
        included.text = "/workspace/web/modules/custom/phpwarden_probe/src/EmptyCoverage.php"
        document.write(native, encoding="unicode")
        empty = run(
            "coverage-empty-source",
            "--coverage",
            "--testsuite=coverage-fixture",
            "--phpunit-config=web/core/phpunit-coverage-native.xml",
            expected=3,
        )
        assert not empty["complete"] and "no executable lines" in empty["diagnostics"][0]
    finally:
        native.unlink(missing_ok=True)
        empty_source.unlink(missing_ok=True)
    configuration = root / "coverage-gate.yaml"
    try:
        configuration.write_text(
            (root / ".phpwarden.yaml").read_text()
            + "\ntests: {coverage: {enabled: true, min_line_coverage: 100, max_crap: 0}}\n"
        )
        gated = run("coverage-threshold", "--config", configuration, expected=1)
        assert gated["complete"] and gated["counts"]["failures"] == 0
        assert len(gated["diagnostics"]) == 2
    finally:
        configuration.unlink()
    try:
        test.write_bytes(original.replace(b"assertGreaterThan(0,", b"assertSame(-1,"))
        failed = run("assertion-failure", expected=1)
        assert failed["complete"] and failed["counts"]["failures"] == 1
        test.write_bytes(
            original.replace(
                b"self::assertGreaterThan(0, ProbeController::hotspot());",
                b"throw new \\RuntimeException('PHPWarden fixture exception');",
            )
        )
        failed = run("test-error", expected=1)
        assert failed["complete"] and failed["counts"]["errors"] >= 1
    finally:
        test.write_bytes(original)
    configuration = root / "web/core/phpunit-broken.xml"
    try:
        configuration.write_bytes(
            (root / "web/core/phpunit.xml.dist")
            .read_bytes()
            .replace(b'bootstrap="tests/bootstrap.php"', b'bootstrap="tests/phpwarden-missing.php"')
        )
        broken = run("bootstrap-error", "--phpunit-config=web/core/phpunit-broken.xml", expected=3)
        assert not broken["complete"] and broken["counts"]["tests"] == 0
    finally:
        configuration.unlink()
    phpunit = root / "vendor/bin/phpunit"
    retained = root / "vendor/bin/phpunit-retained"
    phpunit.rename(retained)
    try:
        missing = run("missing-phpunit", expected=3)
        assert not missing["complete"] and "dev dependencies" in missing["diagnostics"][0]
    finally:
        retained.rename(phpunit)
    assert before == tree_snapshot(root)
    assert baseline == sandbox.git_status()
    sandbox.summary["project_tests"] = results
