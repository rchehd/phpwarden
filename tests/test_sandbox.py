import hashlib
import json
import os
import shutil
import tempfile
import unittest
from copy import deepcopy
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from tools.sandbox.docker import DockerRuntime
from tools.sandbox.host import EXTENSIONS, HostRuntime, extension_configuration, snapshot
from tools.sandbox.run import Sandbox


class SandboxTest(unittest.TestCase):
    def test_host_extensions_use_private_override_and_preserve_dependency_order(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            override, system = root / "override", root / "system"
            override.mkdir()
            system.mkdir()
            builtin = {"pdo", "filter", "zlib"}
            for name in set(EXTENSIONS) - builtin:
                (system / (name + ".so")).touch()
            (override / "mbstring.so").touch()
            lines = extension_configuration(
                {"modules": ["PDO", "filter", "zlib"]}, [override, system]
            )
            self.assertIn("extension=" + json.dumps(str(override / "mbstring.so")), lines)
            self.assertFalse(any("/pdo.so" in line or "spx" in line for line in lines))
            dom = "extension=" + json.dumps(str(system / "dom.so"))
            reader = "extension=" + json.dumps(str(system / "xmlreader.so"))
            self.assertLess(lines.index(dom), lines.index(reader))
            (system / "pdo_sqlite.so").unlink()
            with self.assertRaisesRegex(RuntimeError, "pdo_sqlite"):
                extension_configuration({"modules": list(builtin)}, [override, system])

    def test_host_server_cleanup_stops_owned_process_and_preserves_global_ini(self):
        php = shutil.which("php")
        if php is None:
            self.skipTest("PHP executable unavailable")
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project, logs = root / "project", root / "logs"
            (project / "web").mkdir(parents=True)
            logs.mkdir()
            (project / "web/index.php").write_text("<?php echo 'fixture';\n")
            ini, global_ini = root / "php.ini", root / "global.ini"
            ini.write_text("")
            global_ini.write_text("memory_limit=256M\n")
            sandbox = SimpleNamespace(
                root=root,
                project=project,
                logs=logs,
                environment=os.environ.copy(),
                summary={"host": {}},
            )
            runtime = HostRuntime(sandbox, php)
            runtime.ini, runtime.command = ini, [php, "-n"]
            runtime.global_configuration = snapshot([global_ini])
            try:
                runtime.start()
                before = runtime.application()
                runtime.start()
                self.assertEqual(before, runtime.application())
            finally:
                runtime.cleanup()
            self.assertIsNotNone(runtime.process.poll())
            self.assertTrue(sandbox.summary["host"]["server_stopped"])
            self.assertTrue(sandbox.summary["host"]["global_configuration_preserved"])
            global_ini.write_text("changed\n")
            with self.assertRaisesRegex(AssertionError, "Global PHP configuration changed"):
                runtime.cleanup()

    def test_host_exec_maps_fixture_paths_and_refuses_live_profiling(self):
        sandbox = SimpleNamespace(root=Path("/scratch/data"), project=Path("/scratch/data/project"))
        calls = []
        sandbox.run = lambda command: calls.append(command)
        runtime = HostRuntime(sandbox, shutil.which("python3"))
        runtime.command = ["private-php"]
        runtime.exec(
            [
                "/project/vendor/drush/drush/drush.php",
                "--db-url=sqlite:/data/site.sqlite",
                "--db-url=sqlite://localhost//data/site.sqlite",
            ]
        )
        self.assertEqual(
            calls,
            [
                [
                    "private-php",
                    "/scratch/data/project/vendor/drush/drush/drush.php",
                    "--db-url=sqlite:/scratch/data/data/site.sqlite",
                    "--db-url=sqlite://localhost//scratch/data/data/site.sqlite",
                ]
            ],
        )
        with self.assertRaisesRegex(ValueError, "unsupported"):
            runtime.exec(["-r", "echo 'profile';"], profile=True)
        self.assertEqual(len(calls), 1)

    def test_bare_docker_only_accepts_expected_profile_mounts(self):
        runtime = DockerRuntime(SimpleNamespace(name="fixture", root=Path("/sandbox")))
        reports = "/sandbox/state/owned/reports"
        setup = {
            "image": "phpwarden-spx:fixture",
            "reports": reports,
            "launch_arguments": [
                "--mount",
                f"type=bind,src={reports},dst=/var/lib/phpwarden-spx",
                "--mount",
                "type=bind,src=/sandbox/state/owned/spx.ini,"
                "dst=/usr/local/etc/php/conf.d/99-phpwarden-spx.ini,readonly",
                "phpwarden-spx:fixture",
            ],
        }
        self.assertEqual(runtime.launch_arguments(setup), setup["launch_arguments"][:-1])
        for defect in (
            "extra privilege",
            "socket mount",
            "writable config",
            "other image",
            "outside",
        ):
            value = deepcopy(setup)
            if defect == "extra privilege":
                value["launch_arguments"].insert(0, "--privileged")
            elif defect == "socket mount":
                value["launch_arguments"][1] = "type=bind,src=/var/run/docker.sock,dst=/socket"
            elif defect == "writable config":
                value["launch_arguments"][3] = value["launch_arguments"][3].removesuffix(
                    ",readonly"
                )
            elif defect == "other image":
                value["launch_arguments"][-1] = "other:latest"
            else:
                value["reports"] = "/outside/reports"
            with self.subTest(defect=defect), self.assertRaises(AssertionError):
                runtime.launch_arguments(value)

    def http(self, mode, status="HTTP/1.1 200 OK"):
        sandbox = Sandbox.__new__(Sandbox)
        sandbox.ini, sandbox.http_url = "/fixture.ini", "http://127.0.0.1/fixture"
        sandbox.exec = lambda _arguments: json.dumps(
            {
                "status": status,
                "body": None,
                "preview": "Extension diagnostic before JSON",
            }
        )
        return sandbox.http(mode)

    def test_bad_key_may_emit_notice_but_normal_responses_stay_strict(self):
        self.http("bad")
        for mode in ("on", "off"):
            with self.subTest(mode=mode), self.assertRaises(AssertionError):
                self.http(mode)
        with self.assertRaises(AssertionError):
            self.http("bad", "HTTP/1.1 500 Server Error")

    def test_late_failure_cannot_leave_successful_summary(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository = root / "repository"
            pin = repository / "tools/spx/sources.json"
            pin.parent.mkdir(parents=True)
            source = b"pinned source fixture"
            pin.write_text(
                json.dumps(
                    {
                        "spx": {
                            "url": "https://example.invalid/source",
                            "sha256": hashlib.sha256(source).hexdigest(),
                        }
                    }
                )
            )
            sandbox = Sandbox("10", root / "sandbox", "worker", root / "archive", 1)

            def fail():
                sandbox.summary["passed"] = True
                raise RuntimeError("post-validation failure")

            sandbox.install = fail
            sandbox.cleanup_installer = lambda: None
            with (
                patch("tools.sandbox.run.ROOT", repository),
                patch("tools.sandbox.run.urllib.request.urlopen") as response,
                patch("tools.sandbox.run.subprocess.run"),
            ):
                response.return_value.__enter__.return_value.read.return_value = source
                with self.assertRaisesRegex(RuntimeError, "post-validation failure"):
                    sandbox.execute()
            summary = json.loads((sandbox.root / "summary.json").read_text())
            self.assertFalse(summary["passed"])


if __name__ == "__main__":
    unittest.main()
