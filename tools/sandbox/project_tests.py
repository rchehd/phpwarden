import json
import os
import shutil

from tools.sandbox.assertions import tree_snapshot


def project_tests(sandbox):
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
    assert result["complete"] and result["counts"]["tests"] == 2
    assert result["phpunit_version"].startswith("9." if sandbox.major == "10" else "11.")
    assert not (root / "web/phpwarden-unit-probe").exists()
    filtered = run("filtered", "--filter=testHotspot")
    assert filtered["complete"] and filtered["counts"]["tests"] == 1
    run("empty-filter", "--filter=phpwardenMissingTest", expected=3)
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
