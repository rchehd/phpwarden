import hashlib
import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path

from tools.psalm.prepare import ROOT, prepare


class PsalmBundleTest(unittest.TestCase):
    def test_candidate_preserves_spike_versions_and_actual_license_policy(self):
        lock = json.loads((ROOT / "composer.lock").read_text())
        spike = json.loads((ROOT.parent / "psalm-spike/upstream/composer.lock").read_text())
        self.assertEqual(lock["packages"], spike["packages"])
        self.assertEqual(len(lock["packages"]), 69)
        policy = json.loads((ROOT / "sources.json").read_text())
        self.assertEqual(policy["publication"], "pending-license-decision")
        self.assertEqual(policy["runtime"], "8.3")
        drupal = next(p for p in policy["sources"] if p["name"] == "mortenson/psalm-plugin-drupal")
        self.assertIn("Commons Clause", drupal["license"])
        for notice in policy["additional_notices"]:
            self.assertEqual(
                hashlib.sha256((ROOT / notice["file"]).read_bytes()).hexdigest(), notice["sha256"]
            )

    def fixture(self, root):
        bundle, sources, specification = root / "bundle", root / "sources", root / "policy"
        sources.mkdir()
        specification.mkdir()
        packages, archives = [], []
        for name, license_id, notice in (
            (
                "mortenson/psalm-plugin-drupal",
                "LGPL-2.1-only",
                "Commons Clause License Condition v1.0",
            ),
            ("netresearch/jsonmapper", "OSL-3.0", "Open Software License v. 3.0"),
        ):
            package = {
                "name": name,
                "version": "1.0.0",
                "license": [license_id],
                "source": {"reference": "fixed-revision", "url": "https://example.invalid/source"},
            }
            packages.append(package)
            directory = bundle / "vendor" / name
            directory.mkdir(parents=True)
            data = notice.encode()
            (directory / "LICENSE").write_bytes(data)
            archive = sources / (name.split("/")[1] + ".tar.gz")
            with tarfile.open(archive, "w:gz") as tar:
                entry = tarfile.TarInfo("source/LICENSE")
                entry.size = len(data)
                tar.addfile(entry, io.BytesIO(data))
            archives.append(
                {
                    "name": name,
                    "reference": "fixed-revision",
                    "declared_license": [license_id],
                    "file": archive.name,
                    "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
                }
            )
        (bundle / "composer.lock").write_text(json.dumps({"packages": packages}))
        (specification / "composer.lock").write_bytes((bundle / "composer.lock").read_bytes())
        (bundle / "vendor/composer").mkdir()
        (bundle / "vendor/composer/installed.json").write_text(json.dumps({"packages": packages}))
        (specification / "LGPL.txt").write_text("Supplemental base license text")
        extra = {
            "file": "LGPL.txt",
            "sha256": hashlib.sha256((specification / "LGPL.txt").read_bytes()).hexdigest(),
        }
        (specification / "sources.json").write_text(
            json.dumps(
                {
                    "version": 1,
                    "runtime": "8.3",
                    "publication": "pending-license-decision",
                    "sources": archives,
                    "additional_notices": [extra],
                }
            )
        )
        (specification / "NOTICE.md").write_text("Original license conditions remain applicable.")
        return bundle, sources, specification

    def test_retains_actual_notices_sources_and_pending_publication(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            bundle, sources, policy = self.fixture(root)
            before = {str(p): p.read_bytes() for p in bundle.rglob("*") if p.is_file()}
            output = root / "output"
            report = prepare(bundle, sources, output, specification=policy)
            self.assertEqual(report["publication"], "pending-license-decision")
            self.assertEqual(len(report["packages"]), 2)
            self.assertIn(
                "Commons Clause",
                (output / "notices/mortenson/psalm-plugin-drupal/LICENSE").read_text(),
            )
            self.assertEqual(json.loads((output / "inventory.json").read_text()), report)
            self.assertEqual((output / "LGPL.txt").read_bytes(), (policy / "LGPL.txt").read_bytes())
            for archive in sources.iterdir():
                self.assertEqual(
                    (output / "sources" / archive.name).read_bytes(), archive.read_bytes()
                )
            self.assertEqual(
                before, {str(p): p.read_bytes() for p in bundle.rglob("*") if p.is_file()}
            )
            with self.assertRaisesRegex(ValueError, "empty"):
                prepare(bundle, sources, output, specification=policy)

    def test_rejects_stale_packages_missing_notices_and_changed_archives(self):
        for change in (
            "revision",
            "version",
            "missing-package",
            "duplicate",
            "missing-notice",
            "symlink",
            "source-checksum",
            "license-mismatch",
            "supplemental-notice",
            "lock",
        ):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                bundle, sources, policy = self.fixture(root)
                installed = bundle / "vendor/composer/installed.json"
                rows = json.loads(installed.read_text())
                if change in ("revision", "version"):
                    if change == "revision":
                        rows["packages"][0]["source"]["reference"] = "different"
                    else:
                        rows["packages"][0]["version"] = "2.0.0"
                elif change == "missing-package":
                    rows["packages"].pop()
                elif change == "duplicate":
                    rows["packages"].append(rows["packages"][0])
                elif change == "source-checksum":
                    next(sources.iterdir()).write_bytes(b"changed source")
                elif change == "supplemental-notice":
                    (policy / "LGPL.txt").write_text("Different supplemental terms")
                elif change == "lock":
                    (bundle / "composer.lock").write_text("{}")
                else:
                    notice = bundle / "vendor/mortenson/psalm-plugin-drupal/LICENSE"
                    if change == "license-mismatch":
                        notice.write_text("Different terms")
                    else:
                        notice.unlink()
                        if change == "symlink":
                            notice.symlink_to(policy / "NOTICE.md")
                installed.write_text(json.dumps(rows))
                with self.assertRaises(ValueError):
                    prepare(bundle, sources, root / "output", specification=policy)
                self.assertFalse((root / "output").exists())


if __name__ == "__main__":
    unittest.main()
