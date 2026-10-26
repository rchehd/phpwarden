import hashlib
import json
import os
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path

from tools.distribution import package

ROOT = Path(__file__).resolve().parents[2]


class DistributionCLI(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        for command in ("go", "composer", "npm", "sha256sum", "readlink"):
            if shutil.which(command) is None:
                raise RuntimeError(f"Installation tests require {command}")
        cls.temporary = tempfile.TemporaryDirectory(prefix="phpwarden-install-test-")
        cls.addClassCleanup(cls.temporary.cleanup)
        cls.root = Path(cls.temporary.name)
        cls.environment = os.environ | {
            "COMPOSER_HOME": str(cls.root / "composer-home"),
            "COMPOSER_CACHE_DIR": str(cls.root / "composer-cache"),
            "COMPOSER_DISABLE_NETWORK": "1",
            "npm_config_cache": str(cls.root / "npm-cache"),
            "npm_config_offline": "true",
        }
        cls.artifacts = cls.root / "composer-artifacts"
        cls.artifacts.mkdir()
        cls.releases = {}
        for version in ("v1.0.0", "v1.1.0", "v0.0.0-dev.a38dff3"):
            output = cls.root / version
            output.mkdir()
            images = output / "images.json"
            images.write_text(
                json.dumps(
                    {
                        runtime: "ghcr.io/rchehd/phpwarden@sha256:"
                        + hashlib.sha256((version + runtime).encode()).hexdigest()
                        for runtime in ("8.1", "8.2", "8.3", "8.4")
                    }
                )
            )
            manifest = output / "compatibility.json"
            cls.run_command(
                [
                    "go",
                    "run",
                    "./cmd/phpwarden-dist",
                    "manifest",
                    "--version",
                    version,
                    "--images",
                    str(images),
                    "--output",
                    str(manifest),
                ],
                ROOT,
            )
            package(version, "b" * 40, manifest, output)
            shutil.copy2(
                output / "phpwarden-composer-linux-amd64.zip", cls.artifacts / f"{version}.zip"
            )
            cls.releases[version] = output

    @classmethod
    def run_command(cls, arguments, cwd, expected=0):
        result = subprocess.run(
            [str(x) for x in arguments],
            cwd=cwd,
            env=cls.environment,
            capture_output=True,
            text=True,
            timeout=180,
        )
        if result.returncode != expected:
            raise AssertionError(
                f"{arguments}: expected {expected}, got {result.returncode}\n"
                f"{result.stdout}\n{result.stderr}"
            )
        return result

    def project(self, name):
        root = self.root / name
        root.mkdir()
        self.run_command(["git", "init", "-q"], root)
        (root / "composer.json").write_text(
            json.dumps({"name": "fixture/project", "require": {"drupal/core": "^11"}})
        )
        (root / "composer.lock").write_text(
            json.dumps(
                {
                    "packages": [
                        {"name": "drupal/core", "version": "11.4.1", "require": {"php": ">=8.3"}}
                    ]
                }
            )
        )
        return root

    def verify_wrapper(self, wrapper, root, version):
        result = self.run_command([wrapper, "--version"], root)
        self.assertIn(version, result.stdout)
        self.run_command([wrapper, "init", "-n", "--php-runtime", "8.3", "--pre-commit"], root)
        config = (root / ".phpwarden.yaml").read_bytes()
        self.run_command([wrapper, "init", "-n", "--pre-commit"], root)
        self.assertEqual(config, (root / ".phpwarden.yaml").read_bytes())
        hook = (root / ".git/hooks/pre-commit").read_text()
        self.assertIn("bin/phpwarden", hook)
        self.assertNotIn("payload/phpwarden", hook)
        result = self.run_command([wrapper, "update", "--format=json"], root, expected=3)
        self.assertIn("not self-managed", json.loads(result.stdout)["message"])

    def test_composer_clean_install_reinstall_upgrade_and_rollback(self):
        root = self.project("composer-project")
        manifest = {
            "name": "fixture/project",
            "repositories": [
                {
                    "type": "package",
                    "package": [
                        {
                            "name": "rchehd/phpwarden",
                            "version": self.composer_version(output),
                            "dist": {
                                "type": "zip",
                                "url": (output / "phpwarden-composer-linux-amd64.zip").as_uri(),
                            },
                            "bin": ["bin/phpwarden"],
                        }
                        for version, output in self.releases.items()
                    ],
                },
                {"packagist.org": False},
            ],
            "require-dev": {"rchehd/phpwarden": "1.0.0"},
        }
        (root / "composer.json").write_text(json.dumps(manifest))
        (root / "composer.lock").unlink()
        command = [
            "composer",
            "update",
            "--no-plugins",
            "--no-scripts",
            "--no-interaction",
            "--no-audit",
        ]
        self.run_command(command, root)
        wrapper = root / "vendor/bin/phpwarden"
        self.run_command([wrapper, "--version"], root)
        shutil.rmtree(root / "vendor")
        self.run_command(
            ["composer", "install", "--no-plugins", "--no-scripts", "--no-interaction"], root
        )
        for version in ("1.1.0", "1.0.0"):
            manifest["require-dev"]["rchehd/phpwarden"] = version
            (root / "composer.json").write_text(json.dumps(manifest))
            self.run_command(command, root)
            self.assertIn("v" + version, self.run_command([wrapper, "--version"], root).stdout)
        self.assertEqual(len(json.loads((root / "composer.lock").read_text())["packages-dev"]), 1)
        # Init needs a framework lock; dependency installation itself used only local artifacts.
        (root / "composer.lock").write_text(
            json.dumps(
                {
                    "packages": [
                        {"name": "drupal/core", "version": "11.4.1", "require": {"php": ">=8.3"}}
                    ]
                }
            )
        )
        self.verify_wrapper(wrapper, root, "v1.0.0")
        payload = root / "vendor/rchehd/phpwarden/payload/phpwarden"
        replacement = payload.with_suffix(".tampered")
        replacement.write_bytes(payload.read_bytes() + b"tampered")
        replacement.chmod(0o755)
        replacement.replace(payload)
        result = self.run_command([wrapper, "--version"], root, expected=3)
        self.assertIn("checksum mismatch", result.stderr)

    @staticmethod
    def composer_version(output):
        with zipfile.ZipFile(output / "phpwarden-composer-linux-amd64.zip") as archive:
            return json.loads(archive.read("composer.json"))["version"]

    def test_composer_installs_manual_ci_snapshot_with_original_cli_identity(self):
        version = "v0.0.0-dev.a38dff3"
        output = self.releases[version]
        composer_version = self.composer_version(output)
        self.assertEqual(composer_version, "dev-0.0.0-dev.a38dff3")
        root = self.project("composer-snapshot")
        (root / "composer.json").write_text(
            json.dumps(
                {
                    "name": "fixture/snapshot",
                    "repositories": [
                        {
                            "type": "package",
                            "package": {
                                "name": "rchehd/phpwarden",
                                "version": composer_version,
                                "dist": {
                                    "type": "zip",
                                    "url": (output / "phpwarden-composer-linux-amd64.zip").as_uri(),
                                },
                                "bin": ["bin/phpwarden"],
                            },
                        },
                        {"packagist.org": False},
                    ],
                    "require-dev": {"rchehd/phpwarden": composer_version},
                }
            )
        )
        (root / "composer.lock").unlink()
        self.run_command(
            [
                "composer",
                "update",
                "--no-plugins",
                "--no-scripts",
                "--no-interaction",
                "--no-audit",
            ],
            root,
        )
        wrapper = root / "vendor/bin/phpwarden"
        metadata = json.loads(self.run_command([wrapper, "--build-info"], root).stdout)
        self.assertEqual(metadata["version"], version)
        self.assertEqual(metadata["distribution"], "standalone")
        lock = json.loads((root / "composer.lock").read_text())
        self.assertEqual(lock["packages-dev"][0]["version"], composer_version)

    def test_npm_clean_install_ci_upgrade_and_rollback(self):
        root = self.project("npm-project")
        (root / "package.json").write_text('{"name":"fixture","version":"1.0.0","private":true}')
        for version in ("v1.0.0", "v1.1.0", "v1.0.0"):
            archive = self.releases[version] / "phpwarden-npm-linux-amd64.tgz"
            self.run_command(
                [
                    "npm",
                    "install",
                    "--save-dev",
                    "--save-exact",
                    "--ignore-scripts",
                    "--no-audit",
                    "--no-fund",
                    "--offline",
                    archive,
                ],
                root,
            )
            wrapper = root / "node_modules/.bin/phpwarden"
            self.assertIn(version, self.run_command([wrapper, "--version"], root).stdout)
        lock = (root / "package-lock.json").read_bytes()
        shutil.rmtree(root / "node_modules")
        self.run_command(
            ["npm", "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--offline"], root
        )
        self.assertEqual(lock, (root / "package-lock.json").read_bytes())
        self.verify_wrapper(wrapper, root, "v1.0.0")
        payload = root / "node_modules/@rchehd/phpwarden/payload/phpwarden"
        replacement = payload.with_suffix(".tampered")
        replacement.write_bytes(payload.read_bytes() + b"tampered")
        replacement.chmod(0o755)
        replacement.replace(payload)
        self.assertIn(
            "checksum mismatch", self.run_command([wrapper, "--version"], root, expected=3).stderr
        )

    def test_release_assets_pin_the_same_binary_and_images(self):
        for version, output in self.releases.items():
            self.run_command(["sha256sum", "-c", "SHA256SUMS"], output)
            with zipfile.ZipFile(output / "phpwarden-composer-linux-amd64.zip") as package_file:
                self.assertEqual(
                    package_file.read("payload/phpwarden"), (output / "phpwarden").read_bytes()
                )
                self.assertNotIn("scripts", json.loads(package_file.read("composer.json")))
            with tarfile.open(output / "phpwarden-npm-linux-amd64.tgz") as package_file:
                metadata = json.load(package_file.extractfile("package/package.json"))
                self.assertEqual(metadata["version"], version[1:])
                self.assertNotIn("scripts", metadata)
            images = json.loads((output / "images.json").read_text())
            for runtime, image in images.items():
                with tarfile.open(output / f"phpwarden-compose-php{runtime}.tar.gz") as archive:
                    self.assertEqual(
                        archive.extractfile(".phpwarden-image").read().decode().strip(), image
                    )
                    self.assertEqual(archive.getmember("phpwarden").mode, 0o755)
                    compose = archive.extractfile(".phpwarden.compose.yaml").read()
                    self.assertNotIn(b"docker.sock", compose)


if __name__ == "__main__":
    unittest.main()
