import hashlib
import io
import json
import os
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile
from concurrent.futures import ThreadPoolExecutor
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
    def run_command(cls, arguments, cwd, expected=0, env=None):
        result = subprocess.run(
            [str(x) for x in arguments],
            cwd=cwd,
            env=env or cls.environment,
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
            self.assertEqual(
                (output / "install.sh").read_bytes(), (ROOT / "install.sh").read_bytes()
            )
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

    def install(self, name, version="v1.0.0", extra=(), expected=0, env=None):
        output = self.releases[version]
        directory = self.root / name
        result = self.run_command(
            [
                "/bin/sh",
                ROOT / "install.sh",
                "--archive",
                output / "phpwarden-linux-amd64.tar.gz",
                "--dir",
                directory,
                *extra,
            ],
            self.root,
            expected=expected,
            env=env,
        )
        return directory / "phpwarden", result

    def test_installer_local_archive_without_php_go_docker_or_network(self):
        tools = self.root / "installer-tools"
        tools.mkdir()
        for command in (
            "awk",
            "cat",
            "chmod",
            "cmp",
            "cp",
            "dirname",
            "flock",
            "grep",
            "gzip",
            "head",
            "mkdir",
            "mktemp",
            "mv",
            "pwd",
            "rm",
            "sed",
            "sha256sum",
            "tar",
            "timeout",
            "uname",
            "wc",
        ):
            (tools / command).symlink_to(shutil.which(command))
        env = self.environment | {"PATH": str(tools)}
        for command in ("php", "go", "docker", "curl"):
            self.assertIsNone(shutil.which(command, path=str(tools)))
        binary, result = self.install(
            "installed with spaces", extra=("--version", "1.0.0"), env=env
        )
        self.assertIn("Installed PHPWarden v1.0.0", result.stdout)
        self.assertEqual(binary.read_bytes(), (self.releases["v1.0.0"] / "phpwarden").read_bytes())
        self.assertEqual(binary.stat().st_mode & 0o777, 0o755)
        inode = binary.stat().st_ino
        self.install("installed with spaces", env=env)
        self.assertEqual(binary.stat().st_ino, inode)
        self.assertEqual(list(binary.parent.glob(".phpwarden-install.*")), [])

    def test_installer_force_upgrade_rollback_and_existing_file_preservation(self):
        binary, _ = self.install("installer-replace")
        before = binary.read_bytes()
        self.install("installer-replace", "v1.1.0", expected=3)
        self.assertEqual(binary.read_bytes(), before)
        settings = binary.parent / ".phpwarden.yaml"
        settings.write_text("preserve this project file\n")
        for version in ("v1.1.0", "v1.0.0"):
            self.install("installer-replace", version, extra=("--force",))
            self.assertIn(version, self.run_command([binary, "--version"], self.root).stdout)
        self.assertEqual(settings.read_text(), "preserve this project file\n")
        symlink_dir = self.root / "installer-symlink"
        symlink_dir.mkdir()
        (symlink_dir / "phpwarden").symlink_to(binary)
        self.install("installer-symlink", extra=("--force",), expected=3)
        self.install("vendor/installer-managed", expected=3)
        self.assertEqual(binary.read_bytes(), before)

    def test_installer_concurrent_initial_install_is_idempotent(self):
        with ThreadPoolExecutor(max_workers=2) as executor:
            tasks = [executor.submit(self.install, "installer-concurrent") for _ in range(2)]
            outputs = [task.result()[1].stdout for task in tasks]
        self.assertEqual(sum("already installed" in output for output in outputs), 1)
        self.assertEqual(sum("Installed PHPWarden" in output for output in outputs), 1)

    def test_installer_rejects_unsupported_platform_before_download(self):
        tools = self.root / "installer-platform-tools"
        tools.mkdir()
        uname = tools / "uname"
        uname.write_text("#!/bin/sh\nprintf '%s\\n' Darwin\n")
        uname.chmod(0o755)
        destination = self.root / "unsupported-platform"
        result = self.run_command(
            [
                "/bin/sh",
                ROOT / "install.sh",
                "--dir",
                destination,
            ],
            self.root,
            expected=3,
            env=self.environment
            | {
                "PATH": str(tools),
            },
        )
        self.assertIn("only Linux amd64", result.stderr)
        self.assertFalse(destination.exists())

    def test_installer_rejects_bad_inputs_checksums_and_version_without_replacing(self):
        for args in (
            ["--dir"],
            ["--archive"],
            ["--dir", "--force"],
            ["--version", "01.0.0"],
            ["--version", "v1.0.0-rc.01"],
            ["--version", "v1.0.0\nv2.0.0"],
            ["--version", "../main"],
            ["--force", "--force"],
            ["--checksums", "none"],
            ["--unknown"],
        ):
            with self.subTest(args=args):
                self.run_command(["/bin/sh", ROOT / "install.sh", *args], self.root, expected=2)
        binary, _ = self.install("installer-invalid")
        before = binary.read_bytes()
        self.install("installer-invalid", extra=("--version", "v1.1.0", "--force"), expected=3)
        for contents in (
            "0" * 64 + "  phpwarden-linux-amd64.tar.gz\n",
            (self.releases["v1.0.0"] / "SHA256SUMS").read_text() * 2,
            "0" * 64 + "  other.tar.gz\n",
        ):
            checksums = self.root / "invalid-SHA256SUMS"
            checksums.write_text(contents)
            self.install(
                "installer-invalid", extra=("--checksums", checksums, "--force"), expected=3
            )
            self.assertEqual(binary.read_bytes(), before)

    def test_installer_rejects_unsafe_archive_entries_and_mismatched_metadata(self):
        output = self.releases["v1.0.0"]
        binary, _ = self.install("installer-malformed")
        before = binary.read_bytes()
        for defect in ("traversal", "symlink", "duplicate", "metadata", "manifest"):
            with self.subTest(defect=defect):
                directory = self.root / ("archive-" + defect)
                directory.mkdir()
                archive_path = directory / "phpwarden-linux-amd64.tar.gz"
                with (
                    tarfile.open(output / archive_path.name) as source,
                    tarfile.open(archive_path, "w:gz") as archive,
                ):
                    for info in source:
                        data = source.extractfile(info).read()
                        if defect == "symlink" and info.name == "phpwarden":
                            info.type, info.linkname, info.size = tarfile.SYMTYPE, str(binary), 0
                            archive.addfile(info)
                            continue
                        if defect == "metadata" and info.name == "release.json":
                            metadata = json.loads(data)
                            metadata["commit"] = "c" * 40
                            data = json.dumps(metadata).encode()
                        if defect == "manifest" and info.name == "compatibility.json":
                            data += b" "
                        info.size = len(data)
                        archive.addfile(info, io.BytesIO(data))
                        if defect == "duplicate" and info.name == "phpwarden":
                            archive.addfile(info, io.BytesIO(data))
                    if defect == "traversal":
                        info = tarfile.TarInfo("../escaped")
                        info.size = 4
                        archive.addfile(info, io.BytesIO(b"oops"))
                (directory / "SHA256SUMS").write_text(
                    hashlib.sha256(archive_path.read_bytes()).hexdigest()
                    + "  "
                    + archive_path.name
                    + "\n"
                )
                self.run_command(
                    [
                        "/bin/sh",
                        ROOT / "install.sh",
                        "--archive",
                        archive_path,
                        "--dir",
                        binary.parent,
                        "--force",
                    ],
                    self.root,
                    expected=3,
                )
                self.assertEqual(binary.read_bytes(), before)
                self.assertFalse((self.root / "escaped").exists())

    def test_installer_download_urls_pins_and_failures(self):
        tools = self.root / "installer-download-tools"
        tools.mkdir()
        curl = tools / "curl"
        curl.write_text(
            "#!/usr/bin/python3\n"
            + """import json, os, shutil, sys
from pathlib import Path
args = sys.argv[1:]
with Path(os.environ["INSTALL_TEST_LOG"]).open("a") as stream:
    stream.write(json.dumps(args) + "\\n")
if os.environ.get("INSTALL_TEST_FAIL"):
    raise SystemExit(22)
output = args[args.index("--output") + 1]
name = args[-1].rsplit("/", 1)[-1]
shutil.copyfile(Path(os.environ["INSTALL_TEST_RELEASE"]) / name, output)
"""
        )
        curl.chmod(0o755)
        for pin in (None, "v1.0.0"):
            directory = self.root / ("download-" + str(pin))
            log = self.root / ("download-" + str(pin) + ".jsonl")
            env = self.environment | {
                "PATH": str(tools) + os.pathsep + os.environ["PATH"],
                "INSTALL_TEST_LOG": str(log),
                "INSTALL_TEST_RELEASE": str(self.releases["v1.0.0"]),
                "HOME": str(directory.parent / "isolated-home"),
            }
            args = ["/bin/sh", ROOT / "install.sh", "--dir", directory]
            if pin:
                args += ["--version", pin]
            self.run_command(args, self.root, env=env)
            requests = [json.loads(line) for line in log.read_text().splitlines()]
            path = "latest/download" if pin is None else "download/v1.0.0"
            self.assertEqual(
                [row[-1] for row in requests],
                [
                    f"https://github.com/rchehd/phpwarden/releases/{path}/SHA256SUMS",
                    f"https://github.com/rchehd/phpwarden/releases/{path}/phpwarden-linux-amd64.tar.gz",
                ],
            )
            self.assertTrue(all(row[0] == "-q" and "--proto-redir" in row for row in requests))
        failure_dir = self.root / "download-failure"
        self.run_command(
            [
                "/bin/sh",
                ROOT / "install.sh",
                "--dir",
                failure_dir,
            ],
            self.root,
            expected=3,
            env=env | {"INSTALL_TEST_FAIL": "1"},
        )
        self.assertFalse(failure_dir.exists())

    def test_installer_accepts_development_ci_archive_and_default_user_directory(self):
        output = self.root / "development-cli"
        package("v0.0.0-dev.local", "b" * 40, None, output)
        home = self.root / "installer-home"
        env = self.environment | {"HOME": str(home)}
        result = self.run_command(
            [
                "/bin/sh",
                ROOT / "install.sh",
                "--archive",
                output / "phpwarden-linux-amd64.tar.gz",
            ],
            self.root,
            env=env,
        )
        self.assertIn("preloaded worker images", result.stdout)
        binary = home / ".local/bin/phpwarden"
        metadata = json.loads(self.run_command([binary, "--build-info"], self.root).stdout)
        self.assertEqual(metadata["distribution"], "source")


if __name__ == "__main__":
    unittest.main()
