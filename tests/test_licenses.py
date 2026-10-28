import hashlib
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tools.licenses import generate, json_stream, notices


class LicenseInventoryTest(unittest.TestCase):
    def test_notice_text_normalizes_line_endings_but_hashes_original_source(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            raw = b"Upstream license\r\nSecond line\r\n"
            (root / "LICENSE").write_bytes(raw)
            module = {
                "Path": "fixture/module",
                "Version": "v1.0.0",
                "Sum": "h1:pinned",
                "Dir": str(root),
            }
            responses = [
                json.dumps({"Module": module}),
                json.dumps({"GOROOT": str(root), "GOVERSION": "go1.25.0"}),
            ]
            with (
                patch("tools.licenses.go", side_effect=responses),
                patch("tools.licenses.subprocess.run"),
            ):
                output = generate()
            self.assertNotIn(b"\r", output["THIRD_PARTY_NOTICES.txt"])
            records = json.loads(output["dependencies.json"])["components"]
            self.assertEqual(
                records[1]["license_files"][0]["sha256"], hashlib.sha256(raw).hexdigest()
            )

    def test_stream_rejects_truncated_module_data(self):
        values = list(json_stream(' {"Path":"first"}\n {"Path":"second"}\n'))
        self.assertEqual([v["Path"] for v in values], ["first", "second"])
        with self.assertRaises(json.JSONDecodeError):
            list(json_stream('{"Path":"first"}\n{"Path":'))

    def test_module_subcomponent_notices_are_retained(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "LICENSE").write_text("Primary license")
            child = root / "internal/decoder"
            child.mkdir(parents=True)
            (child / "NOTICE.txt").write_text("Bundled component notice")
            (child / "decoder.go").write_text("package decoder")
            self.assertEqual(
                {p.relative_to(root).as_posix() for p in notices(root)},
                {"LICENSE", "internal/decoder/NOTICE.txt"},
            )

    def test_new_module_without_license_fails_explicitly(self):
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(ValueError):
                notices(Path(directory))


if __name__ == "__main__":
    unittest.main()
