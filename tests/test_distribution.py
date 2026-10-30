import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tools.distribution import (
    REPOSITORY,
    RUNTIMES,
    check_package_output,
    package,
    published_digests,
)


class DistributionTest(unittest.TestCase):
    def test_published_manifest_reads_tested_images_and_exact_digests(self):
        calls = []

        def run(arguments, **_options):
            calls.append(arguments)
            digest = "a" * 64
            return subprocess.CompletedProcess(
                arguments,
                0,
                stdout=json.dumps([{"RepoDigests": [f"{REPOSITORY}@sha256:{digest}"]}]),
            )

        result = published_digests(REPOSITORY, run)
        self.assertEqual(set(result), set(RUNTIMES))
        self.assertEqual(len(calls), 4)
        self.assertTrue(all(call[:3] == ["docker", "image", "inspect"] for call in calls))

    def test_missing_or_ambiguous_repository_digests_fail(self):
        for digests in (
            [],
            ["foreign/image@sha256:" + "a" * 64],
            [
                REPOSITORY + "@sha256:" + "a" * 64,
                REPOSITORY + "@sha256:" + "b" * 64,
            ],
        ):
            with self.subTest(digests=digests):

                def run(arguments, digests=digests, **_options):
                    return subprocess.CompletedProcess(
                        arguments, 0, stdout=json.dumps([{"RepoDigests": digests}])
                    )

                with self.assertRaises(ValueError):
                    published_digests(REPOSITORY, run)

    def test_packaging_rejects_invalid_version_and_commit_before_build(self):
        for version, commit in (
            ("v1.0.0;echo", "a" * 40),
            ("v1.0.0", "main"),
            ("v01.0.0", "a" * 40),
            ("v1.0.0-rc.01", "a" * 40),
            ("v1.0.0-dev.0000001", "a" * 40),
            ("v1.0.0-dev..a", "a" * 40),
            ("v1.0.0-" + "a" * 128, "a" * 40),
        ):
            with self.subTest(version=version), patch("tools.distribution.subprocess.run") as run:
                with self.assertRaises(ValueError):
                    package(version, commit, None, None)
                run.assert_not_called()

    def test_packaging_preserves_occupied_outputs_before_building(self):
        for name in (
            "phpwarden",
            "phpwarden-linux-amd64.tar.gz",
            "phpwarden-composer-linux-amd64.zip",
            "phpwarden-npm-linux-amd64.tgz",
            "phpwarden-compose-php8.0.tar.gz",
            "compatibility.json",
            "SHA256SUMS",
            "install.sh",
        ):
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / "dist [old]"
                output.mkdir()
                existing = output / name
                existing.write_bytes(b"preserve old artifact")
                with patch("tools.distribution.subprocess.run") as run:
                    with self.assertRaisesRegex(ValueError, "fresh directory"):
                        package("v1.2.3", "a" * 40, None, output)
                    run.assert_not_called()
                self.assertEqual(existing.read_bytes(), b"preserve old artifact")

    def test_packaging_refuses_symlink_output_and_allows_explicit_manifest_input(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "dist"
            output.mkdir()
            manifest = output / "compatibility.json"
            manifest.write_text("input manifest")
            check_package_output(output, manifest)
            alias = Path(directory) / "alias"
            alias.symlink_to(output)
            with self.assertRaisesRegex(ValueError, "symlink"):
                check_package_output(alias, manifest)
            manifest.unlink()
            manifest.symlink_to(Path(directory) / "missing")
            with self.assertRaisesRegex(ValueError, "fresh directory"):
                check_package_output(output, manifest)

    def test_numeric_commit_snapshot_version_reaches_build(self):
        with tempfile.TemporaryDirectory() as directory:
            with patch(
                "tools.distribution.subprocess.run", side_effect=RuntimeError("build probe")
            ) as run:
                with self.assertRaisesRegex(RuntimeError, "build probe"):
                    package("v0.0.0-dev.g0000001", "0000001" + "a" * 33, None, Path(directory))
                self.assertEqual(run.call_args.args[0][:2], ["go", "build"])


if __name__ == "__main__":
    unittest.main()
