import json
import subprocess
import unittest

from tools.distribution import REPOSITORY, RUNTIMES, package, published_digests


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
        for version, commit in (("v1.0.0;echo", "a" * 40), ("v1.0.0", "main")):
            with self.assertRaises(ValueError):
                package(version, commit, None, None)


if __name__ == "__main__":
    unittest.main()
