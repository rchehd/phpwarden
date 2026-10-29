import json

from tools.sandbox.assertions import tree_snapshot


def mutation_tests(sandbox):
    root = sandbox.root / "unit-project"
    module = root / "web/modules/custom/phpwarden_probe"
    source = module / "src/CoverageProbe.php"
    test = module / "tests/src/Unit/SplitATest.php"
    sandbox.run(["git", "init", "--quiet"], cwd=root)
    sandbox.run(["git", "add", "--force", "--", "web/modules/custom/phpwarden_probe"], cwd=root)
    sandbox.run(
        [
            "git",
            "-c",
            "core.hooksPath=/dev/null",
            "-c",
            "commit.gpgSign=false",
            "-c",
            "user.name=PHPWarden",
            "-c",
            "user.email=phpwarden@localhost",
            "commit",
            "--quiet",
            "--allow-empty",
            "--no-verify",
            "-m",
            "Mutation fixture base",
        ],
        cwd=root,
    )
    before = tree_snapshot(root)
    source_original, test_original = source.read_bytes(), test.read_bytes()
    results = {}
    native = root / "mutation-native.json5"
    gate = root / "mutation-gate.yaml"

    def run(name, *arguments, expected=0):
        output = sandbox.evidence.log_directory() / ("mutation-" + name)
        result = json.loads(
            sandbox.run(
                [
                    sandbox.binary,
                    "mutation",
                    "--project",
                    root,
                    "--format=json",
                    "--output",
                    output,
                    *arguments,
                ],
                cwd=root,
                expected=(expected,),
                timeout=600,
            )
        )
        assert result["command"] == "mutation" and result["exit_code"] == expected
        assert json.loads((output / "report.json").read_text()) == result
        results[name] = {"exit_code": expected, "complete": result["complete"]}
        if result.get("mutation"):
            results[name]["stats"] = result["mutation"]["stats"]
            assert (output / "infection.html").is_file()
            assert (output / "infection-coverage/coverage-xml/index.xml").is_file()
        return result

    try:
        strong = run("strong", source)
        assert strong["complete"] and strong["mutation"]["stats"]["msi"] == 100
        assert strong["mutation"]["stats"]["killedCount"] > 0
        test.write_bytes(test_original.replace(b"assertSame(1,", b"assertGreaterThan(0,"))
        weak = run("weak", source)
        assert weak["complete"] and weak["mutation"]["stats"]["escapedCount"] > 0
        assert weak["mutation"]["escaped"][0]["diff"]
        gate.write_text((root / ".phpwarden.yaml").read_text() + "\nmutation: {min_msi: 100}\n")
        gated = run("threshold", "--config", gate, source, expected=1)
        assert gated["complete"] and gated["counts"]["failures"] == 0
        test.write_bytes(test_original)
        uncovered = run("uncovered", module / "src/UncoveredProbe.php")
        stats = uncovered["mutation"]["stats"]
        assert uncovered["complete"] and stats["totalMutantsCount"] == stats["notCoveredCount"]
        gate.write_text(
            (root / ".phpwarden.yaml").read_text() + "\nmutation: {min_covered_msi: 100}\n"
        )
        missing = run(
            "no-covered-mutations", "--config", gate, module / "src/UncoveredProbe.php", expected=3
        )
        assert not missing["complete"]
        native.write_text(
            "{source: {directories: ['web/modules/custom']}, threads: 2, "
            "mutators: {'@default': false, IncrementInteger: true},}\n"
        )
        configured = run("native", "--infection-config", native.name, source)
        assert configured["complete"]
        rows = configured["mutation"]["killed"]
        assert rows and all(
            row["mutator"]["mutatorName"].endswith("IncrementInteger") for row in rows
        )
        source.write_bytes(source_original.replace(b"return 1;", b"return 2;"))
        test.write_bytes(test_original.replace(b"assertSame(1,", b"assertSame(2,"))
        diff = run("diff", "--base=HEAD")
        whole = run("whole-files", "--base=HEAD", "--whole-files")
        assert diff["complete"] and whole["complete"]
        assert diff["selection"]["mutation"]["scope"]["mode"] == "diff"
        assert (
            diff["mutation"]["stats"]["totalMutantsCount"]
            < whole["mutation"]["stats"]["totalMutantsCount"]
        )
        assert all(
            row["mutator"]["originalFilePath"].endswith("CoverageProbe.php")
            for row in diff["mutation"]["killed"]
        )
        test.write_bytes(test_original)
        initial = run("initial-failure", source, expected=1)
        assert not initial["complete"] and not initial.get("mutation")
        source.write_bytes(source_original)
        native.write_text(
            "{source: {directories: ['web/modules/custom']}, mutators: {'@default': false}}"
        )
        empty = run("no-mutators", "--infection-config", native.name, source, expected=3)
        assert not empty["complete"]
        native.write_text(
            "{source: {directories: ['web/modules/custom'], excludes: ['{CoverageProbe.php$}']}}"
        )
        excluded = run("excluded", "--infection-config", native.name, source, expected=3)
        assert not excluded["complete"]
        added = module / "src/AddedMutation.php"
        try:
            added.write_text(
                "<?php\nnamespace Drupal\\phpwarden_probe;\n"
                "final class AddedMutation {\n"
                "  public function value(): int { return 1; }\n}\n"
            )
            new = run("added-diff", "--base=HEAD")
            assert new["complete"] and new["mutation"]["stats"]["notCoveredCount"] > 0
            assert new["selection"]["mutation"]["scope"]["files"] == [
                "web/modules/custom/phpwarden_probe/src/AddedMutation.php"
            ]
        finally:
            added.unlink(missing_ok=True)
        source.write_bytes(
            source_original.replace(
                b"return 1;",
                b"$i = 0;\n      while ($i < 2) { ++$i; }\n      return 1;",
            )
        )
        gate.write_text(
            (root / ".phpwarden.yaml").read_text()
            + "\nmutation: {mutant_timeout_seconds: 1, mutators: [Increment]}\n"
        )
        timed = run("timeout", "--config", gate, source, expected=3)
        assert not timed["complete"] and timed["mutation"]["stats"]["timeOutCount"] > 0
    finally:
        source.write_bytes(source_original)
        test.write_bytes(test_original)
        native.unlink(missing_ok=True)
        gate.unlink(missing_ok=True)
    assert before == tree_snapshot(root)
    sandbox.summary["mutation_tests"] = results
