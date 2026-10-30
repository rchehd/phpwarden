import hashlib
import io
import json
import shutil
import tarfile
import zipfile
from copy import deepcopy

from tools.distribution import package
from tools.sandbox.assertions import preserved


class PackageInstallations:
    def __init__(self, sandbox):
        self.sandbox = sandbox
        self.root = sandbox.root / "packages"
        self.root.mkdir()
        self.releases = []
        commit = sandbox.run(["git", "rev-parse", "HEAD"], cwd=sandbox.repository).strip()
        for number in (1, 2):
            output = self.root / str(number)
            version = f"v0.0.0-dev.g{commit[:7]}.{number}"
            package(version, commit, None, output)
            composer = output / "phpwarden-composer-linux-amd64.zip"
            npm = output / "phpwarden-npm-linux-amd64.tgz"
            with zipfile.ZipFile(composer) as archive:
                composer_version = json.loads(archive.read("composer.json"))["version"]
                assert "compatibility.json" not in archive.namelist()
            with tarfile.open(npm) as archive:
                assert "package/compatibility.json" not in archive.getnames()
            self.releases.append(
                {
                    "version": version,
                    "composer_version": composer_version,
                    "composer": composer,
                    "npm": npm,
                }
            )

    def composer(self, *arguments):
        sandbox = self.sandbox
        if sandbox.summary["environment"] == "host":
            return sandbox.runtime.composer(*arguments, offline=True)
        return sandbox.run(
            [
                "docker",
                "run",
                "--rm",
                "--name",
                sandbox.name + "-install",
                "--network",
                "none",
                "--user",
                sandbox.user,
                "--mount",
                f"type=bind,src={sandbox.project},dst=/project",
                "--mount",
                f"type=bind,src={self.root},dst=/artifacts,readonly",
                "--workdir",
                "/project",
                "--env",
                "COMPOSER_HOME=/tmp/composer-home",
                "--env",
                "COMPOSER_CACHE_DIR=/tmp/composer-cache",
                "--env",
                "COMPOSER_DISABLE_NETWORK=1",
                "--entrypoint",
                "php",
                sandbox.image,
                "/usr/local/lib/phpwarden/composer/vendor/bin/composer",
                *arguments,
            ],
            timeout=180,
        )

    def install_composer(self, release, initial=False):
        sandbox = self.sandbox
        path = sandbox.project / "composer.json"
        manifest = json.loads(path.read_text())
        if initial:
            self.original_manifest = deepcopy(manifest)
            self.original_lock = json.loads((sandbox.project / "composer.lock").read_text())
            self.installed_packages = json.loads(
                (sandbox.project / "vendor/composer/installed.json").read_text()
            )["packages"]
            repository = {
                "type": "package",
                "package": [
                    {
                        "name": "rchehd/phpwarden",
                        "version": row["composer_version"],
                        "dist": {
                            "type": "zip",
                            "url": (
                                row["composer"].as_uri()
                                if sandbox.summary["environment"] == "host"
                                else "/artifacts/"
                                + row["composer"].relative_to(self.root).as_posix()
                            ),
                            "shasum": hashlib.sha1(row["composer"].read_bytes()).hexdigest(),
                        },
                        "bin": ["bin/phpwarden"],
                    }
                    for row in self.releases
                ],
            }
            repositories = manifest.get("repositories", [])
            if not isinstance(repositories, list):
                raise ValueError("Sandbox Composer repositories must be an array")
            manifest["repositories"] = [repository, *repositories]
        manifest.setdefault("require-dev", {})["rchehd/phpwarden"] = release["composer_version"]
        path.write_text(json.dumps(manifest, indent=2))
        self.composer(
            "update",
            "rchehd/phpwarden",
            "--no-scripts",
            "--no-interaction",
            "--no-audit",
            "--prefer-dist",
        )
        sandbox.binary = sandbox.project / "vendor/bin/phpwarden"
        self.preserve_composer()

    def preserve_composer(self):
        sandbox = self.sandbox
        manifest = json.loads((sandbox.project / "composer.json").read_text())
        lock = json.loads((sandbox.project / "composer.lock").read_text())
        assert lock["packages"] == self.original_lock["packages"]
        installed = json.loads((sandbox.project / "vendor/composer/installed.json").read_text())
        assert [p for p in installed["packages"] if p["name"] != "rchehd/phpwarden"] == (
            self.installed_packages
        )
        assert [
            p for p in lock["packages-dev"] if p["name"] != "rchehd/phpwarden"
        ] == self.original_lock["packages-dev"]
        for key, value in self.original_manifest.items():
            if key not in ("repositories", "require-dev"):
                assert manifest[key] == value, key
        assert manifest["repositories"][1:] == self.original_manifest.get("repositories", [])
        assert {
            k: v for k, v in manifest["require-dev"].items() if k != "rchehd/phpwarden"
        } == self.original_manifest.get("require-dev", {})

    def reinstall_composer(self):
        lock = (self.sandbox.project / "composer.lock").read_bytes()
        self.composer(
            "reinstall",
            "rchehd/phpwarden",
            "--no-scripts",
            "--no-interaction",
            "--prefer-dist",
        )
        assert lock == (self.sandbox.project / "composer.lock").read_bytes()
        self.preserve_composer()

    def install_npm(self, release, initial=False):
        sandbox = self.sandbox
        path = sandbox.project / "package.json"
        if initial:
            if path.exists():
                raise ValueError("Sandbox npm project already exists")
            self.npm_manifest = {
                "name": "phpwarden-sandbox",
                "private": True,
                "description": "Preserve existing frontend settings",
                "scripts": {
                    "postinstall": (
                        "node -e \"require('fs').writeFileSync('unexpected-script', 'ran')\""
                    )
                },
            }
            path.write_text(json.dumps(self.npm_manifest, indent=2))
            frontend = self.root / "frontend.tgz"
            with tarfile.open(frontend, "w:gz") as archive:
                for name, data in {
                    "package/package.json": b'{"name":"@fixture/frontend","version":"1.0.0"}',
                    "package/index.js": b"module.exports = 'preserved frontend dependency';\n",
                }.items():
                    entry = tarfile.TarInfo(name)
                    entry.size = len(data)
                    archive.addfile(entry, io.BytesIO(data))
            sandbox.run(
                [
                    "npm",
                    "install",
                    "--save-exact",
                    "--ignore-scripts",
                    "--no-audit",
                    "--no-fund",
                    "--offline",
                    frontend,
                ]
            )
            self.npm_manifest = json.loads(path.read_text())
            self.frontend_lock = json.loads((sandbox.project / "package-lock.json").read_text())[
                "packages"
            ]["node_modules/@fixture/frontend"]
        sandbox.run(
            [
                "npm",
                "install",
                "--save-dev",
                "--save-exact",
                "--ignore-scripts",
                "--no-audit",
                "--no-fund",
                "--offline",
                release["npm"],
            ]
        )
        sandbox.binary = sandbox.project / "node_modules/.bin/phpwarden"
        self.preserve_npm()

    def preserve_npm(self):
        sandbox = self.sandbox
        manifest = json.loads((sandbox.project / "package.json").read_text())
        for key, value in self.npm_manifest.items():
            assert manifest[key] == value, key
        lock = json.loads((sandbox.project / "package-lock.json").read_text())
        assert lock["packages"]["node_modules/@fixture/frontend"] == self.frontend_lock
        assert (sandbox.project / "node_modules/@fixture/frontend/index.js").read_text() == (
            "module.exports = 'preserved frontend dependency';\n"
        )
        assert not (sandbox.project / "unexpected-script").exists()

    def reinstall_npm(self):
        sandbox = self.sandbox
        lock = (sandbox.project / "package-lock.json").read_bytes()
        shutil.rmtree(sandbox.project / "node_modules")
        sandbox.run(["npm", "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--offline"])
        assert lock == (sandbox.project / "package-lock.json").read_bytes()
        self.preserve_npm()

    def verify(self, release, manager):
        sandbox = self.sandbox
        metadata = json.loads(sandbox.cli("--build-info"))
        assert metadata["version"] == release["version"] and metadata["distribution"] == "source"
        assert "manifest_sha256" not in metadata
        blocked = json.loads(sandbox.cli("update", "--format=json", expected=(3,)))
        assert "not self-managed" in blocked["message"]
        hook = (sandbox.project / ".git/hooks/pre-commit").read_text()
        assert hook.encode() == sandbox.precommit_contents
        assert (sandbox.project / ".phpwarden.yaml").read_bytes() == self.config
        wrapper = (
            "vendor/rchehd/phpwarden/bin/phpwarden"
            if manager == "composer"
            else "node_modules/@rchehd/phpwarden/bin/phpwarden"
        )
        assert (sandbox.project / wrapper).is_file()
        assert wrapper in hook and "payload/phpwarden" not in hook
        result = json.loads(
            sandbox.cli(
                "check", "--mode=full", "--only=syntax,debug", "--no-cache", "--format=json"
            )
        )
        assert result["complete"]
        sandbox.run([sandbox.project / ".git/hooks/pre-commit"])
        if manager == "composer":
            self.preserve_composer()
        else:
            self.preserve_npm()

    def run(self, manager):
        sandbox = self.sandbox
        sandbox.stage(f"checking full-site {manager} install/reinstall/upgrade/rollback")
        install = self.install_composer if manager == "composer" else self.install_npm
        reinstall = self.reinstall_composer if manager == "composer" else self.reinstall_npm
        first, second = self.releases
        install(first, initial=True)
        sandbox.project_checks(manager)
        config = (sandbox.project / ".phpwarden.yaml").read_bytes()
        self.config = config
        self.verify(first, manager)
        reinstall()
        self.verify(first, manager)
        for release in (second, first):
            install(release)
            self.verify(release, manager)
            with preserved([sandbox.project / ".phpwarden.yaml"]):
                sandbox.cli("init", "-n", "--agents=all", "--hooks=all", "--pre-commit")
        sandbox.checkpoint(f"Verified {manager} upgrades")
        sandbox.summary.setdefault("installations", {})[manager] = {
            "installed": True,
            "reinstalled": True,
            "upgrade": second["version"],
            "rollback": first["version"],
            "hooks_preserved": True,
        }
