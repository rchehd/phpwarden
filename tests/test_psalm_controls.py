import importlib.util
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("psalm_spike", ROOT / "tools/psalm-spike/drupal.py")
SPIKE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SPIKE)


class PsalmControlsTest(unittest.TestCase):
    def test_renamed_hook_and_monorepo_trace(self):
        with tempfile.TemporaryDirectory() as directory:
            project = Path(directory)
            hook = project / "web/modules/custom/catalog/catalog.module"
            hook.parent.mkdir(parents=True)
            hook.write_text(
                "<?php\nfunction catalog_preprocess_html(&$variables) {\n echo 'x';\n}\n"
            )
            location = {
                "file_name": "../project/apps/site/web/modules/custom/catalog/catalog.module",
                "file_path": "/project/apps/site/web/modules/custom/catalog/catalog.module",
                "line_from": 3,
            }
            issues = [{"type": "TaintedHtml", "taint_trace": [location]}]
            options = {
                "module": "catalog",
                "labels": {"hook"},
                "container_root": "/project/apps/site",
            }
            self.assertEqual(SPIKE.controls(project, issues, **options), {"hook": ["TaintedHtml"]})
            location["file_path"] = "/project/unrelated/catalog.module"
            location["file_name"] = "modules/custom/catalog/catalog.module"
            self.assertEqual(SPIKE.controls(project, issues, **options), {"hook": []})


if __name__ == "__main__":
    unittest.main()
