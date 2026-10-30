import hashlib
import io
import json
import os
import shutil
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SETUP = ROOT / "internal/agentskills/skills/phpwarden-setup/scripts/setup.sh"
INSTALLER = ROOT / "install.sh"
TOOLS = (
    "awk cat chmod cmp cp dirname flock grep gzip head mkdir mktemp mv pwd rm sed "
    "sha256sum sh tar timeout tr uname wc"
).split()


class PluginSetupTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="phpwarden-plugin-setup-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.project = self.root / "project [fixture]"
        self.project.mkdir()
        self.helpers, self.home, self.scratch = [
            self.root / name for name in ("helpers", "home", "tmp")
        ]
        for directory in (self.helpers, self.home, self.scratch):
            directory.mkdir()
        for name in TOOLS:
            (self.helpers / name).symlink_to(shutil.which(name))
        self.log = self.root / "calls"
        self.environment = os.environ | {
            "PATH": str(self.helpers),
            "HOME": str(self.home),
            "TMPDIR": str(self.scratch),
            "BOOT_CALL_LOG": str(self.log),
        }
        (self.project / "AGENTS.md").write_text("preserved instructions")

    def cli(self, path, version="v1.2.3", published=True, commit="a" * 40):
        metadata = {
            "version": version,
            "commit": commit,
            "distribution": "standalone" if published else "source",
            "platform": "linux/amd64",
        }
        manifest = b'{"release":"' + version.encode() + b'"}\n'
        if published:
            metadata["manifest_sha256"] = hashlib.sha256(manifest).hexdigest()
        identity = json.dumps(metadata, indent=2) + "\n"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            '#!/bin/sh\nprintf "%s\\n" "$*" >> "$BOOT_CALL_LOG"\n'
            "case \"${1:-}\" in --build-info) cat <<'IDENTITY'\n"
            + identity
            + "IDENTITY\n;; *) exit 9 ;; esac\n"
        )
        path.chmod(0o755)
        return metadata, manifest, path.read_bytes()

    def archive(self, published=True, commit="a" * 40):
        directory = self.root / ("published" if published else "source")
        directory.mkdir()
        metadata, manifest, binary = self.cli(
            directory / "fixture-cli", published=published, commit=commit
        )
        archive = directory / "phpwarden-linux-amd64.tar.gz"
        files = {
            "phpwarden": binary,
            "release.json": (json.dumps(metadata, indent=2) + "\n").encode(),
            "COMMIT": (commit + "\n").encode(),
            "README.md": b"fixture\n",
        }
        if published:
            files["compatibility.json"] = manifest
            (directory / "compatibility.json").write_bytes(manifest)
        with tarfile.open(archive, "w:gz") as output:
            for name, data in files.items():
                entry = tarfile.TarInfo(name)
                entry.size, entry.mode = len(data), 0o755 if name == "phpwarden" else 0o644
                output.addfile(entry, io.BytesIO(data))
        (directory / "SHA256SUMS").write_text(
            hashlib.sha256(archive.read_bytes()).hexdigest() + "  " + archive.name + "\n"
        )
        return directory, binary

    def run_setup(self, action, *arguments, expected=0, cwd=None):
        result = subprocess.run(
            ["/bin/sh", str(SETUP), action, "--project", str(self.project), *map(str, arguments)],
            cwd=cwd or self.project,
            env=self.environment,
            capture_output=True,
            text=True,
            timeout=45,
        )
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        self.assertEqual(list(self.scratch.iterdir()), [])
        self.assertEqual((self.project / "AGENTS.md").read_text(), "preserved instructions")
        self.assertFalse((self.project / ".phpwarden.yaml").exists())
        return result

    def gh(self, artifact, *, workflow="Build", conclusion="success", commit="a" * 40):
        gh = self.helpers / "gh"
        gh.write_text(
            '#!/bin/sh\nprintf "gh %s\\n" "$*" >> "$BOOT_CALL_LOG"\n'
            'case "$1 $2" in\n"run view") cat <<\'METADATA\'\n'
            + f"{workflow}\t{commit}\tcompleted\t{conclusion}\nMETADATA\n;;\n"
            + '"run download") while [ "$#" -gt 0 ]; do\n'
            'if [ "$1" = --dir ]; then destination=$2; shift 2; else shift; fi\ndone\n'
            'mkdir -p "$destination"\ncp "$TEST_ARTIFACT/"* "$destination/"\n'
            ";;\n*) exit 8 ;; esac\n"
        )
        gh.chmod(0o755)
        self.environment["TEST_ARTIFACT"] = str(artifact)

    def test_missing_status_does_not_create_client_or_runtime_state(self):
        result = self.run_setup("status", expected=3)
        self.assertIn("CLI: missing", result.stdout)
        self.assertIn("Project config: missing", result.stdout)
        self.assertFalse(self.log.exists())
        self.assertEqual(list(self.home.iterdir()), [])

    def test_discovery_prefers_managed_project_cli_then_path_then_user(self):
        candidates = (
            (self.project / "vendor/bin/phpwarden", "v1.0.0", "composer"),
            (self.project / "node_modules/.bin/phpwarden", "v2.0.0", "npm"),
            (self.helpers / "phpwarden", "v3.0.0", "PATH"),
            (self.home / ".local/bin/phpwarden", "v4.0.0", "user"),
        )
        for path, version, _ in candidates:
            self.cli(path, version)
        for path, version, source in candidates:
            result = self.run_setup("status")
            self.assertIn("Version: " + version, result.stdout)
            self.assertIn("Source: " + source, result.stdout)
            path.unlink()
        self.assertEqual(self.log.read_text().splitlines(), ["--build-info"] * 4)

    def test_explicit_cli_and_broken_managed_cli_never_silently_fall_back(self):
        explicit = self.root / "explicit-cli"
        self.cli(explicit, "v5.0.0")
        self.cli(self.helpers / "phpwarden")
        self.assertIn("Version: v5.0.0", self.run_setup("status", "--cli", explicit).stdout)
        managed = self.project / "vendor/bin/phpwarden"
        managed.parent.mkdir(parents=True)
        managed.write_text("not executable")
        self.assertIn("not executable", self.run_setup("status", expected=3).stderr)
        self.assertIn(
            "missing", self.run_setup("status", "--cli", self.root / "missing", expected=3).stderr
        )

    def test_offline_install_and_repeat_reuse_without_replacing_or_downloading(self):
        directory, binary = self.archive()
        result = self.run_setup(
            "install",
            "--archive",
            directory / "phpwarden-linux-amd64.tar.gz",
            "--version",
            "v1.2.3",
        )
        self.assertIn("published manifest", result.stdout)
        installed = self.home / ".local/bin/phpwarden"
        self.assertEqual(installed.read_bytes(), binary)
        inode = installed.stat().st_ino
        self.run_setup("install", "--ci-run", "123")
        self.assertEqual(installed.stat().st_ino, inode)
        self.assertNotIn("gh", self.log.read_text())
        self.assertIn(
            "another version", self.run_setup("install", "--version", "v2.0.0", expected=3).stderr
        )
        self.assertEqual(installed.read_bytes(), binary)

    def test_source_archive_is_reported_as_preloaded_worker_only(self):
        directory, _ = self.archive(published=False)
        result = self.run_setup("install", "--archive", directory / "phpwarden-linux-amd64.tar.gz")
        self.assertIn("preloaded development images required", result.stdout)

    def test_relative_archive_and_destination_stay_relative_to_invocation(self):
        self.archive()
        self.run_setup(
            "install",
            "--archive",
            "published/phpwarden-linux-amd64.tar.gz",
            "--checksums",
            "published/SHA256SUMS",
            "--dir",
            "custom-bin",
            cwd=self.root,
        )
        self.assertTrue((self.root / "custom-bin/phpwarden").is_file())
        self.assertFalse((self.project / "custom-bin").exists())

    def test_bad_checksum_never_installs(self):
        directory, _ = self.archive()
        (directory / "SHA256SUMS").write_text("0" * 64 + "  phpwarden-linux-amd64.tar.gz\n")
        self.assertIn(
            "checksum mismatch",
            self.run_setup(
                "install", "--archive", directory / "phpwarden-linux-amd64.tar.gz", expected=3
            ).stderr,
        )
        self.assertFalse((self.home / ".local").exists())

    def test_release_download_uses_canonical_installer_and_preserves_failed_install(self):
        directory, binary = self.archive()
        curl = self.helpers / "curl"
        curl.write_text(
            '#!/bin/sh\nprintf "curl %s\\n" "$*" >> "$BOOT_CALL_LOG"\n'
            '[ "${FAIL_DOWNLOAD:-0}" = 0 ] || exit 22\n'
            'while [ "$#" -gt 0 ]; do\n'
            'if [ "$1" = --output ]; then target=$2; shift 2; else url=$1; shift; fi\ndone\n'
            'cp "$TEST_ARTIFACT/${url##*/}" "$target"\n'
        )
        curl.chmod(0o755)
        self.environment["TEST_ARTIFACT"] = str(directory)
        self.environment["FAIL_DOWNLOAD"] = "1"
        self.run_setup("install", "--version", "v1.2.3", expected=3)
        self.assertFalse((self.home / ".local").exists())
        self.environment["FAIL_DOWNLOAD"] = "0"
        self.run_setup("install", "--version", "v1.2.3")
        self.assertEqual((self.home / ".local/bin/phpwarden").read_bytes(), binary)
        self.assertIn(
            "https://github.com/rchehd/phpwarden/releases/download/v1.2.3/", self.log.read_text()
        )
        self.assertIn("--proto =https --proto-redir =https", self.log.read_text())

    def test_install_reuses_managed_cli_without_touching_user_installation(self):
        managed = self.project / "vendor/bin/phpwarden"
        self.cli(managed)
        before = managed.read_bytes()
        result = self.run_setup("install", "--ci-run", "123")
        self.assertIn("Source: composer", result.stdout)
        self.assertEqual(managed.read_bytes(), before)
        self.assertEqual(list(self.home.iterdir()), [])
        self.assertEqual(self.log.read_text().splitlines(), ["--build-info"])

    def test_successful_exact_ci_candidate_uses_official_repo_and_commit(self):
        directory, binary = self.archive()
        self.gh(directory)
        self.run_setup("install", "--ci-run", "123")
        self.assertEqual((self.home / ".local/bin/phpwarden").read_bytes(), binary)
        log = self.log.read_text()
        self.assertIn("--repo rchehd/phpwarden", log)
        self.assertIn("--name phpwarden-linux-amd64-" + "a" * 40, log)

    def test_wrong_ci_commit_is_rejected_before_installing(self):
        directory, _ = self.archive(commit="b" * 40)
        self.gh(directory)
        result = self.run_setup("install", "--ci-run", "123", expected=3)
        self.assertIn("source commit differs", result.stderr)
        self.assertFalse((self.home / ".local").exists())

    def test_cli_only_and_forged_outer_manifest_candidates_are_rejected(self):
        directory, _ = self.archive(published=False)
        self.gh(directory)
        self.assertIn(
            "no published worker manifest",
            self.run_setup("install", "--ci-run", "123", expected=3).stderr,
        )
        (directory / "compatibility.json").write_text("not an archive manifest")
        self.assertIn(
            "published worker manifest is required",
            self.run_setup("install", "--ci-run", "123", expected=3).stderr,
        )
        self.assertFalse((self.home / ".local").exists())

    def test_unsuccessful_ci_source_does_not_download(self):
        directory, _ = self.archive()
        self.gh(directory, conclusion="failure")
        self.run_setup("install", "--ci-run", "123", expected=3)
        self.assertNotIn("run download", self.log.read_text())

    def test_unsupported_platform_stops_before_ci_or_installer(self):
        (self.helpers / "uname").unlink()
        (self.helpers / "uname").write_text("#!/bin/sh\nprintf '%s\\n' Darwin\n")
        (self.helpers / "uname").chmod(0o755)
        self.run_setup("install", "--ci-run", "123", expected=3)
        self.assertFalse(self.log.exists())
        self.assertEqual(list(self.home.iterdir()), [])

    def test_invalid_options_do_not_write(self):
        for action, arguments in (
            ("status", ["--version", "v1.0.0"]),
            ("install", ["--force"]),
            ("install", ["--ci-run", "../123"]),
            ("status", ["--cli"]),
            ("status", ["--project", "bad\npath"]),
            ("install", ["--ci-run", "123", "--archive", "file"]),
        ):
            with self.subTest(arguments=arguments):
                self.run_setup(action, *arguments, expected=2)

    def test_canonical_installer_commit_input_is_strict(self):
        result = subprocess.run(
            ["/bin/sh", str(INSTALLER), "--commit", "a" * 40 + "\nextra"],
            env=self.environment,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertEqual(list(self.scratch.iterdir()), [])
