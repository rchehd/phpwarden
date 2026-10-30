import errno
import json
import os
import pty
import select
import shutil
import subprocess
import tempfile
import time
import unittest
from pathlib import Path

from tests.support.composer import (
    check_composer_audit,
    check_composer_validate_fixtures,
    check_composer_validate_settings,
)
from tests.support.deptrac import check_deptrac
from tests.support.drupal_config import (
    check_drupal_config_custom_layout,
    check_drupal_config_fixtures,
    check_drupal_config_settings,
)
from tests.support.eventhorizon import check_eventhorizon, check_eventhorizon_config
from tests.support.fix import check_fix
from tests.support.gitleaks import check_gitleaks_fixtures, check_strict_rules
from tests.support.node import check_node_no_config, check_node_tools
from tests.support.packagecheck import check_package_tools
from tests.support.phpcs import (
    check_phpcs_fixtures,
    check_phpcs_project_rules,
    check_phpcs_settings,
)
from tests.support.phpmd import TOOLS, check_phpmd_fixtures, check_phpmd_settings
from tests.support.phpstan import (
    check_phpstan_dependency_mismatch,
    check_phpstan_fixtures,
    check_phpstan_project_config,
    check_phpstan_settings,
)
from tests.support.rector import (
    check_rector,
    check_rector_profile_preset,
    check_rector_project_config,
)
from tests.support.scenarios import materialize
from tests.support.semgrep import check_semgrep, check_semgrep_project_rules
from tests.support.syntax import check_parallel_syntax, check_syntax_fixtures, check_syntax_settings
from tests.support.twig_cs_fixer import (
    check_twig_cs_fixer_fixtures,
    check_twig_cs_fixer_project_config,
    check_twig_cs_fixer_settings,
)
from tests.support.whitespace import check_whitespace_fixtures, check_whitespace_settings
from tests.support.yaml import FIXTURES as YAML_FIXTURES
from tests.support.yaml import check_yaml_fixtures, check_yaml_settings

ROOT = Path(__file__).resolve().parents[2]
FIXTURES = ROOT / "tests" / "fixtures" / "scenarios"


class WorkerCLITest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="phpwarden-cli-")
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.binary = Path(cls.temporary.name) / "bin" / "phpwarden"
        cls.binary.parent.mkdir()
        tools = Path(cls.temporary.name) / "lib" / "phpwarden" / "yaml"
        tools.parent.mkdir(parents=True)
        tools.symlink_to(ROOT / "tools" / "yaml", target_is_directory=True)
        (tools.parent / "phpcs").symlink_to(ROOT / "tools" / "phpcs", target_is_directory=True)
        gitleaks = shutil.which("gitleaks")
        if gitleaks:
            (cls.binary.parent / "gitleaks").symlink_to(gitleaks)
        (tools.parent / "node").symlink_to(ROOT / "tools" / "node", target_is_directory=True)
        (tools.parent / "eventhorizon").symlink_to(
            ROOT / "tools" / "eventhorizon", target_is_directory=True
        )
        for bundle in ("composer-require-checker", "composer-unused", "parallel-lint"):
            (tools.parent / bundle).symlink_to(ROOT / "tools" / bundle, target_is_directory=True)
        (tools.parent / "deptrac").symlink_to(ROOT / "tools" / "deptrac", target_is_directory=True)
        (tools.parent / "rector").symlink_to(ROOT / "tools" / "rector", target_is_directory=True)
        (tools.parent / "semgrep").symlink_to(ROOT / "tools" / "semgrep", target_is_directory=True)
        (tools.parent / "phpmd").symlink_to(ROOT / "tools" / "phpmd", target_is_directory=True)
        (tools.parent / "phpstan").symlink_to(ROOT / "tools" / "phpstan", target_is_directory=True)
        (tools.parent / "composer").symlink_to(
            ROOT / "tools" / "composer", target_is_directory=True
        )
        (tools.parent / "twig-cs-fixer").symlink_to(
            ROOT / "tools" / "twig-cs-fixer", target_is_directory=True
        )
        subprocess.run(
            ["go", "build", "-o", str(cls.binary), "./cmd/phpwarden"],
            cwd=ROOT,
            check=True,
            capture_output=True,
            timeout=120,
        )
        cls.runtime = subprocess.run(
            ["php", "-r", 'echo PHP_MAJOR_VERSION . "." . PHP_MINOR_VERSION;'],
            check=True,
            capture_output=True,
            text=True,
            timeout=10,
        ).stdout

    def configure(self, scenario, **debug):
        config = {
            "version": 1,
            "profile": "drupal",
            "php": {
                "runtime": self.runtime,
                "target": {"min": self.runtime, "max": self.runtime},
            },
            "scope": {"include": ["web/modules/custom"], "base": scenario.base},
            "tools": {
                "debug": debug,
                "composer-audit": {"enabled": False},
                "composer-validate": {"enabled": False},
                "phpcs": {"enabled": False},
                "phpmd": {"enabled": False},
                "eslint": {"enabled": False},
                "gitleaks": {"enabled": False},
                "stylelint": {"enabled": False},
                "jscpd": {"enabled": False},
                "phpstan": {"enabled": False},
                "twig-cs-fixer": {"enabled": False},
                "semgrep": {"enabled": False},
                "eventhorizon": {"enabled": False},
                "dead-code": {"enabled": False},
                "rector": {"enabled": False},
                "deptrac": {"enabled": False},
            },
        }
        (scenario.root / ".phpwarden.yaml").write_text(json.dumps(config))

    def run_worker(self, scenario, arguments=(), env=None):
        completed = subprocess.run(
            [str(self.binary), "worker", "check", "--format", "json", *arguments],
            cwd=scenario.root,
            env=env or scenario.env,
            capture_output=True,
            text=True,
            timeout=60,
        )
        self.assertEqual(completed.stderr, "", completed.stderr)
        report = json.loads(completed.stdout)
        self.assertEqual(completed.returncode, report["exit_code"], report)
        self.assertEqual(report["version"], 1)
        self.assertTrue(report["run_id"])
        return report

    def test_real_worker_matches_fixture_expectations(self):
        for directory in sorted(FIXTURES.iterdir()):
            for mode in ("diff", "full", "whole-files", "paths"):
                with self.subTest(scenario=directory.name, mode=mode):
                    with materialize(directory) as scenario:
                        self.configure(scenario)
                        arguments = {
                            "diff": [],
                            "full": ["--mode", "full"],
                            "whole-files": ["--whole-files"],
                            "paths": scenario.spec["changed_files"],
                        }[mode]
                        report = self.run_worker(scenario, arguments)
                        expected_mode = "diff" if mode == "diff" else "full"
                        verdict = scenario.spec["verdicts"][expected_mode]
                        self.assertEqual(report["verdict"], verdict, report)
                        self.assertEqual(report["exit_code"], 1 if verdict == "fail" else 0)
                        self.assertTrue(report["complete"], report)
                        self.assertEqual(report["diagnostics"], [])
                        expected = sorted(
                            (row["checker"], row["path"], row["line"])
                            for row in scenario.spec["findings"]
                            if expected_mode in row["modes"]
                        )
                        actual = sorted(
                            (row["checker"], row["path"], row["line"]) for row in report["findings"]
                        )
                        self.assertEqual(actual, expected)
                        self.assertFalse((scenario.root / "vendor").exists())

    def test_disabled_checker_inventory(self):
        with materialize(FIXTURES / "clean-debug") as scenario:
            self.configure(scenario)
            filename = scenario.root / ".phpwarden.yaml"
            config = json.loads(filename.read_text())
            config["tools"] = {name: {"enabled": False} for name in TOOLS}
            filename.write_text(json.dumps(config))
            report = self.run_worker(scenario, ["--mode", "full"])
            self.assertEqual([row["id"] for row in report["checks"]], list(TOOLS))
            self.assertEqual(report["exit_code"], 0, report)
            self.assertTrue(report["complete"], report)
            self.assertEqual(report["findings"], [])
            self.assertEqual(report["diagnostics"], [])
            for row in report["checks"]:
                self.assertEqual((row["status"], row["reason"]), ("skipped", "disabled"))

    def test_parallel_analyzers_preserve_real_results(self):
        for directory in sorted(FIXTURES.iterdir()):
            with self.subTest(scenario=directory.name), materialize(directory) as scenario:
                self.configure(scenario)
                path = scenario.root / ".phpwarden.yaml"
                config = json.loads(path.read_text())
                baseline = None
                for concurrency in (1, 2, 0):
                    config["runner"] = {"concurrency": concurrency}
                    path.write_text(json.dumps(config))
                    report = self.run_worker(scenario, ["--mode", "full"])
                    actual = {
                        key: report[key]
                        for key in (
                            "verdict",
                            "exit_code",
                            "complete",
                            "checks",
                            "findings",
                            "diagnostics",
                            "environment",
                        )
                    }
                    if baseline is None:
                        baseline = actual
                        self.assertTrue(report["complete"], report)
                    else:
                        self.assertEqual(actual, baseline, f"concurrency={concurrency}")

    def test_syntax_findings_include_errors_outside_diff(self):
        check_syntax_fixtures(self, self.configure, self.run_worker)

    def test_syntax_settings_and_explicit_selection(self):
        check_syntax_settings(self, self.configure, self.run_worker)

    def test_yaml_fixtures(self):
        check_yaml_fixtures(self, self.configure, self.run_worker)

    def test_yaml_settings(self):
        check_yaml_settings(self, self.configure, self.run_worker)

    def test_whitespace_fixtures(self):
        check_whitespace_fixtures(self, self.configure, self.run_worker)

    def test_whitespace_settings(self):
        check_whitespace_settings(self, self.configure, self.run_worker)

    def test_drupal_config_fixtures(self):
        check_drupal_config_fixtures(self, self.configure, self.run_worker)

    def test_drupal_config_settings(self):
        check_drupal_config_settings(self, self.configure, self.run_worker)

    def test_drupal_config_custom_layout(self):
        check_drupal_config_custom_layout(self, self.configure, self.run_worker)

    def test_phpcs_fixtures(self):
        check_phpcs_fixtures(self, self.configure, self.run_worker)

    def test_phpcs_settings(self):
        check_phpcs_settings(self, self.configure, self.run_worker)

    def test_phpcs_project_rules(self):
        check_phpcs_project_rules(self, self.configure, self.run_worker)

    def test_phpstan_fixtures(self):
        check_phpstan_fixtures(self, self.configure, self.run_worker)

    def test_phpstan_settings(self):
        check_phpstan_settings(self, self.configure, self.run_worker)

    def test_phpstan_project_config(self):
        check_phpstan_project_config(self, self.configure, self.run_worker)

    def test_phpstan_dependency_mismatch(self):
        check_phpstan_dependency_mismatch(self, self.configure, self.run_worker)

    def test_twig_cs_fixer_fixtures(self):
        check_twig_cs_fixer_fixtures(self, self.configure, self.run_worker)

    def test_twig_cs_fixer_settings(self):
        check_twig_cs_fixer_settings(self, self.configure, self.run_worker)

    def test_twig_cs_fixer_project_config(self):
        check_twig_cs_fixer_project_config(self, self.configure, self.run_worker)

    def test_package_dependency_tools(self):
        check_package_tools(self, self.configure, self.run_worker)

    def test_parallel_syntax(self):
        check_parallel_syntax(self, self.configure, self.run_worker)

    def test_deptrac_architecture(self):
        check_deptrac(self, self.configure, self.run_worker)

    def test_rector_dry_run(self):
        check_rector(self, self.configure, self.run_worker)

    def test_rector_profile_preset(self):
        check_rector_profile_preset(self, self.configure, self.run_worker)

    def test_rector_project_config(self):
        check_rector_project_config(self, self.configure, self.run_worker)

    def test_semgrep_security(self):
        check_semgrep(self, self.configure, self.run_worker)

    def test_semgrep_project_rules(self):
        check_semgrep_project_rules(self, self.configure, self.run_worker)

    def test_eventhorizon_caching(self):
        check_eventhorizon(self, self.configure, self.run_worker)

    def test_eventhorizon_config(self):
        check_eventhorizon_config(self, self.configure, self.run_worker)

    def test_fix_and_recheck(self):
        check_fix(self, self.configure, self.run_worker)

    def test_gitleaks_fixtures(self):
        check_gitleaks_fixtures(self, self.configure, self.run_worker)

    def test_phpstan_strict_rules(self):
        check_strict_rules(self, self.configure, self.run_worker)

    def test_node_tools(self):
        check_node_tools(self, self.configure, self.run_worker)

    def test_node_no_config(self):
        check_node_no_config(self, self.configure, self.run_worker)

    def test_phpmd_fixtures(self):
        check_phpmd_fixtures(self, self.configure, self.run_worker)

    def test_phpmd_settings(self):
        check_phpmd_settings(self, self.configure, self.run_worker)

    def test_composer_validate_fixtures(self):
        check_composer_validate_fixtures(self, self.configure, self.run_worker)

    def test_composer_validate_settings(self):
        check_composer_validate_settings(self, self.configure, self.run_worker)

    def test_composer_audit(self):
        check_composer_audit(self, self.configure, self.run_worker, network=True)

    def test_missing_yaml_bundle_never_loads_project_autoload(self):
        with materialize(YAML_FIXTURES / "yaml-duplicate-key") as scenario:
            self.configure(scenario)
            with tempfile.TemporaryDirectory(prefix="phpwarden-no-tools-") as temporary:
                binary = Path(temporary) / "bin" / "phpwarden"
                binary.parent.mkdir()
                shutil.copy2(self.binary, binary)
                vendor = scenario.root / "vendor"
                vendor.mkdir()
                (vendor / "autoload.php").write_text(
                    "<?php file_put_contents(__DIR__ . '/../executed', 'bad');\n"
                )
                for required in (True, False):
                    filename = scenario.root / ".phpwarden.yaml"
                    config = json.loads(filename.read_text())
                    config["tools"]["yaml"] = {"required": required}
                    filename.write_text(json.dumps(config))
                    completed = subprocess.run(
                        [str(binary), "worker", "check", "--only", "yaml", "--format", "json"],
                        cwd=scenario.root,
                        env=scenario.env,
                        capture_output=True,
                        text=True,
                        timeout=60,
                    )
                    result = json.loads(completed.stdout)
                    self.assertEqual(completed.returncode, 3 if required else 0, result)
                    self.assertFalse(result["complete"])
                    self.assertEqual(result["checks"][0]["reason"], "dependencies_missing")
                self.assertFalse((scenario.root / "executed").exists())

    def test_process_failures_and_optional_policy(self):
        for name, script, settings, code, status in (
            ("missing", None, {}, 3, "unavailable"),
            ("malformed", "echo '{}'", {}, 3, "error"),
            ("crash", "exit 70", {}, 3, "error"),
            ("timeout", "exec /bin/sleep 5", {"timeout_seconds": 1}, 3, "error"),
            ("optional", None, {"required": False}, 0, "unavailable"),
        ):
            with self.subTest(case=name), materialize(FIXTURES / "added-debug") as scenario:
                self.configure(scenario, **settings)
                filename = scenario.root / ".phpwarden.yaml"
                config = json.loads(filename.read_text())
                config["tools"]["syntax"] = {"enabled": False}
                filename.write_text(json.dumps(config))
                with tempfile.TemporaryDirectory(prefix="phpwarden-path-") as temporary:
                    directory = Path(temporary)
                    (directory / "git").symlink_to(shutil.which("git"))
                    if script is not None:
                        executable = directory / "php"
                        executable.write_text("#!/bin/sh\n" + script + "\n")
                        executable.chmod(0o755)
                    env = scenario.env | {"PATH": str(directory)}
                    report = self.run_worker(scenario, env=env)
                    self.assertEqual(report["exit_code"], code, report)
                    self.assertFalse(report["complete"])
                    self.assertEqual(
                        next(row for row in report["checks"] if row["id"] == "debug")["status"],
                        status,
                    )
                    self.assertTrue(report["diagnostics"])

    def test_explicit_selection_overrides_disabled_checker(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario, enabled=False)
            report = self.run_worker(scenario)
            self.assertEqual(report["exit_code"], 0)
            self.assertEqual(
                next(row for row in report["checks"] if row["id"] == "debug")["reason"], "disabled"
            )
            report = self.run_worker(scenario, ["--only", "debug"])
            self.assertEqual(report["exit_code"], 1)
            report = self.run_worker(scenario, ["--only", "phpstna"])
            self.assertEqual(report["exit_code"], 2)
            self.assertEqual(report["verdict"], "error")

    def test_init_check_selection_and_unattended_defaults(self):
        for arguments, ci, answers, prompt, code, selected, agents in (
            ([], "", b"none\nnone\nnone\ndebug,rector\n\n", True, 0, {"debug", "rector"}, False),
            ([], "", b"\n\n\n", True, 0, None, False),
            ([], "", b"all\n\n\n", True, 0, None, True),
            (["--no-interaction"], "", None, False, 0, None, False),
            (["-n"], "", None, False, 0, None, False),
            (["-n", "--agents", "all"], "", None, False, 0, None, True),
            (["--format", "json"], "", None, False, 0, None, False),
            ([], "true", None, False, 0, None, False),
            (["--pre-commit"], "", b"all\nq\n", True, 130, None, False),
            ([], "", b"\x04", True, 130, None, False),
        ):
            with (
                self.subTest(arguments=arguments, ci=ci, answers=answers),
                materialize(FIXTURES / "added-debug") as scenario,
            ):
                (scenario.root / "composer.json").write_text("{}")
                (scenario.root / "composer.lock").write_text(
                    json.dumps({"packages": [{"name": "drupal/core", "version": "11.4.10"}]})
                )
                master, slave = pty.openpty()
                process = subprocess.Popen(
                    [str(self.binary), "init", "--php-runtime", self.runtime, *arguments],
                    cwd=scenario.root,
                    env=scenario.env | {"TERM": "xterm", "CI": ci},
                    stdin=slave,
                    stdout=slave,
                    stderr=slave,
                )
                os.close(slave)
                output = bytearray()
                sent = False
                deadline = time.monotonic() + 20
                try:
                    while True:
                        remaining = deadline - time.monotonic()
                        self.assertGreater(remaining, 0, "PTY init did not finish")
                        if not select.select([master], [], [], min(remaining, 1))[0]:
                            continue
                        try:
                            chunk = os.read(master, 65536)
                        except OSError as error:
                            if error.errno == errno.EIO:
                                break
                            raise
                        if not chunk:
                            break
                        output.extend(chunk)
                        if answers is not None and not sent and b"\n> " in output:
                            os.write(master, answers)
                            sent = True
                    self.assertEqual(process.wait(timeout=5), code, output.decode())
                finally:
                    os.close(master)
                    if process.poll() is None:
                        process.kill()
                        process.wait(timeout=5)
                text = output.decode()
                self.assertEqual("Install project skills for agents?" in text, prompt, text)
                if code == 0:
                    self.assertEqual("Choose checks for this project" in text, prompt, text)
                config_path = scenario.root / ".phpwarden.yaml"
                skill_path = scenario.root / ".agents/skills/phpwarden-architecture"
                alias = scenario.root / ".claude/skills/phpwarden-architecture"
                self.assertEqual(skill_path.exists(), agents, text)
                self.assertEqual(alias.is_symlink(), agents, text)
                if agents:
                    self.assertEqual(alias.resolve(), skill_path, text)
                    self.assertTrue((skill_path / "SKILL.md").is_file(), text)
                    for name in ("review", "doctor", "upgrade", "performance", "tests"):
                        shared = scenario.root / f".agents/skills/phpwarden-{name}"
                        linked = scenario.root / f".claude/skills/phpwarden-{name}"
                        self.assertTrue((shared / "SKILL.md").is_file(), text)
                        self.assertEqual(linked.resolve(), shared, text)
                    reference = (
                        scenario.root / ".claude/skills/phpwarden-tests/references/reports.md"
                    )
                    self.assertTrue(reference.is_file(), text)
                if code == 130:
                    self.assertFalse(config_path.exists(), text)
                    self.assertFalse((scenario.root / ".git/hooks/pre-commit").exists(), text)
                    continue
                show = subprocess.run(
                    [str(self.binary), "config", "show", "--format", "json"],
                    cwd=scenario.root,
                    env=scenario.env,
                    capture_output=True,
                    text=True,
                    timeout=10,
                    check=True,
                )
                tools = json.loads(show.stdout)["effective"]["config"]["tools"]
                if selected is None:
                    self.assertTrue(tools["rector"]["enabled"], text)
                    self.assertTrue(tools["deptrac"]["enabled"], text)
                    self.assertFalse(tools["composer-unused"]["enabled"], text)
                    self.assertFalse(tools["composer-require-checker"]["enabled"], text)
                else:
                    self.assertEqual(
                        {key for key, value in tools.items() if value["enabled"]}, selected, text
                    )

    def test_deptrac_default_skips_without_architecture_rules(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario)
            path = scenario.root / ".phpwarden.yaml"
            config = json.loads(path.read_text())
            config["tools"].pop("deptrac")
            path.write_text(json.dumps(config))
            probe = scenario.root / "web/modules/custom/demo/ArchitectureProbe.php"
            probe.parent.mkdir(parents=True, exist_ok=True)
            probe.write_text("<?php\nfinal class ArchitectureProbe {}\n")
            report = self.run_worker(scenario, [probe.relative_to(scenario.root).as_posix()])
            check = next(row for row in report["checks"] if row["id"] == "deptrac")
            self.assertEqual(check["status"], "skipped", report)
            self.assertEqual(check["reason"], "no_config", report)
            self.assertTrue(
                any(row["code"] == "architecture_config_missing" for row in report["diagnostics"]),
                report,
            )

    def test_progress_is_interactive_text_only(self):
        for output_format, term, ci, interactive in (
            ("text", "xterm", "", True),
            ("json", "xterm", "", False),
            ("compact", "xterm", "", False),
            ("text", "dumb", "", False),
            ("text", "xterm", "true", False),
        ):
            with (
                self.subTest(format=output_format, term=term, ci=ci),
                materialize(FIXTURES / "added-debug") as scenario,
            ):
                self.configure(scenario)
                master, slave = pty.openpty()
                process = subprocess.Popen(
                    [
                        str(self.binary),
                        "worker",
                        "check",
                        "--only",
                        "whitespace",
                        "--mode",
                        "full",
                        "--format",
                        output_format,
                    ],
                    cwd=scenario.root,
                    env=scenario.env | {"TERM": term, "CI": ci},
                    stdout=slave,
                    stderr=slave,
                )
                os.close(slave)
                output = bytearray()
                deadline = time.monotonic() + 30
                try:
                    while True:
                        remaining = deadline - time.monotonic()
                        self.assertGreater(remaining, 0, "PTY worker did not finish")
                        if not select.select([master], [], [], min(remaining, 1))[0]:
                            continue
                        try:
                            chunk = os.read(master, 65536)
                        except OSError as error:
                            if error.errno == errno.EIO:
                                break
                            raise
                        if not chunk:
                            break
                        output.extend(chunk)
                    self.assertEqual(process.wait(timeout=5), 0, output.decode())
                finally:
                    os.close(master)
                    if process.poll() is None:
                        process.kill()
                        process.wait(timeout=5)
                text = output.decode()
                self.assertEqual("\x1b[2K" in text, interactive, text)
                if interactive:
                    self.assertLess(text.rfind("\x1b[2K"), text.index("Result:"), text)
                if output_format == "json":
                    self.assertTrue(json.loads(text)["complete"], text)

    def test_runtime_mismatch_is_blocking(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario, required=False)
            filename = scenario.root / ".phpwarden.yaml"
            config = json.loads(filename.read_text())
            config["php"]["runtime"] = "99.0"
            filename.write_text(json.dumps(config))
            report = self.run_worker(scenario)
            self.assertEqual(report["exit_code"], 3)
            self.assertIn("runtime_mismatch", [row["code"] for row in report["diagnostics"]])

    def test_checker_settings_change_real_results(self):
        with materialize(FIXTURES / "added-debug") as scenario:
            self.configure(scenario, functions=["print_r"])
            self.assertEqual(self.run_worker(scenario)["findings"], [])
            self.configure(scenario, fail_on="never")
            report = self.run_worker(scenario)
            self.assertEqual(report["exit_code"], 0)
            self.assertEqual(len(report["findings"]), 1)
            self.assertTrue(report["complete"])
            self.configure(scenario, exclude=["**/*.module"])
            report = self.run_worker(scenario)
            self.assertEqual(report["exit_code"], 0)
            self.assertEqual(
                next(row for row in report["checks"] if row["id"] == "debug")["reason"], "no_files"
            )


if __name__ == "__main__":
    unittest.main()
