import json
import tempfile
import unittest
from pathlib import Path

from tools.licenses import json_stream, notices


class LicenseInventoryTest(unittest.TestCase):
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
