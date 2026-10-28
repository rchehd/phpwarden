import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tools.sandbox.run import Sandbox


class SandboxTest(unittest.TestCase):
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
