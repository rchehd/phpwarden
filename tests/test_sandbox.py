import hashlib
import json
import tempfile
import unittest
from copy import deepcopy
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from tools.sandbox.docker import DockerRuntime
from tools.sandbox.run import Sandbox


class SandboxTest(unittest.TestCase):
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
