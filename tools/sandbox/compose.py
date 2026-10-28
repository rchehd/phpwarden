import hashlib
import json
import os
import shutil


class ComposeInstallation:
    def __init__(self, sandbox):
        self.sandbox = sandbox
        self.host = sandbox.binary
        self.wrapper = sandbox.project / "phpwarden"
        self.quality_project = (
            "phpwarden-" + hashlib.sha256(str(sandbox.project).encode()).hexdigest()[:12]
        )
        self.controller = (
            sandbox.project
            / "web/modules/custom/phpwarden_probe/src/Controller/ProbeController.php"
        )

    def image(self):
        sandbox = self.sandbox
        context = sandbox.root / "compose-worker"
        context.mkdir()
        commit = sandbox.run(["git", "rev-parse", "HEAD"], cwd=sandbox.repository).strip()
        version = "v0.0.0-dev." + commit[:7]
        environment = sandbox.environment
        sandbox.environment = environment | {"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"}
        try:
            sandbox.run(
                [
                    "go",
                    "build",
                    "-trimpath",
                    "-ldflags",
                    f"-s -w -X github.com/rchehd/phpwarden.Distribution=container "
                    f"-X github.com/rchehd/phpwarden.Version={version} "
                    f"-X github.com/rchehd/phpwarden.Commit={commit}",
                    "-o",
                    context / "phpwarden",
                    "./cmd/phpwarden",
                ],
                cwd=sandbox.repository,
            )
        finally:
            sandbox.environment = environment
        base = sandbox.name + ":compose-base"
        (context / "Dockerfile").write_text(
            f"FROM {base}\nCOPY --chmod=755 phpwarden /usr/local/bin/phpwarden\n"
            f'LABEL org.opencontainers.image.version="{version}" '
            f'org.opencontainers.image.revision="{commit}"\n'
        )
        tag = sandbox.name + ":compose-worker"
        sandbox.run(["docker", "tag", sandbox.worker_id, base])
        try:
            sandbox.run(
                ["docker", "build", "--pull=false", "--network=none", "--tag", tag, context]
            )
        finally:
            sandbox.run(["docker", "image", "rm", base])
        return sandbox.run(["docker", "image", "inspect", tag, "--format", "{{.Id}}"]).strip()

    def application(self):
        sandbox = self.sandbox
        containers = sandbox.compose("ps", "-q", "php").split()
        assert len(containers) == 1, containers
        sandbox.container = containers[0]
        inspection = json.loads(sandbox.run(["docker", "inspect", sandbox.container]))[0]
        assert inspection["State"]["Running"]
        return {
            key: inspection[key]
            for key in ("Id", "Image", "Config", "HostConfig", "Mounts", "RestartCount")
        } | {
            "started_at": inspection["State"]["StartedAt"],
        }

    def database(self):
        return self.sandbox.exec(
            [
                "-r",
                '$db=new PDO("sqlite:/data/site.sqlite");$db->exec("PRAGMA query_only=ON");'
                '$rows=$db->query("SELECT collection,name,hex(data) FROM config '
                'ORDER BY collection,name")->fetchAll(PDO::FETCH_NUM);'
                'echo hash("sha256",json_encode($rows,JSON_THROW_ON_ERROR));',
            ]
        ).strip()

    def snapshot(self, names):
        root = self.sandbox.project
        return {
            name: (
                (root / name).read_bytes(),
                (root / name).stat().st_uid,
                (root / name).stat().st_gid,
            )
            for name in names
        }

    def failures(self):
        sandbox = self.sandbox
        configuration = sandbox.project / ".phpwarden.yaml"
        original = configuration.read_bytes()
        try:
            for arguments in (
                ("init", "-n", "--hooks=all"),
                ("doctor",),
                ("update",),
                ("performance", "--help"),
            ):
                sandbox.cli(*arguments, expected=(2,))
                assert configuration.read_bytes() == original
            configuration.write_bytes(original.replace(b'runtime: "8.3"', b'runtime: "8.4"'))
            assert configuration.read_bytes() != original
            mismatch = json.loads(
                sandbox.cli("check", "--mode=full", "--only=syntax", "--format=json", expected=(3,))
            )
            assert not mismatch["complete"]
        finally:
            configuration.write_bytes(original)
        hook = sandbox.project / ".git/hooks/pre-commit"
        original_hook = hook.read_bytes()
        try:
            hook.write_text("#!/bin/sh\nexit 17\n")
            sandbox.cli("init", "-n", "--pre-commit", expected=(2,))
            assert hook.read_bytes() == b"#!/bin/sh\nexit 17\n"
            assert configuration.read_bytes() == original
        finally:
            hook.write_bytes(original_hook)
        image = sandbox.project / ".phpwarden-image"
        original_image = image.read_bytes()
        try:
            image.write_text("untrusted:latest\n")
            sandbox.cli("--version", expected=(2,))
        finally:
            image.write_bytes(original_image)

    def staged_mismatch(self):
        sandbox = self.sandbox
        config = sandbox.project / ".phpwarden.yaml"
        original_config, original = config.read_bytes(), self.controller.read_bytes()
        effective = json.loads(sandbox.cli("config", "show", "--format=json"))["effective"][
            "config"
        ]
        simple = {
            "version": 1,
            "profile": "drupal",
            "php": effective["php"],
            "scope": {"base": "HEAD"},
            "tools": {
                name: {"enabled": name in ("syntax", "debug")} for name in effective["tools"]
            },
        }
        try:
            config.write_text(json.dumps(simple))
            self.controller.write_bytes(original + b"\n")
            sandbox.run(["git", "add", self.controller])
            self.controller.write_bytes(original + b"\n\n")
            sandbox.run([sandbox.project / ".git/hooks/pre-commit"], expected=(2,))
        finally:
            config.write_bytes(original_config)
            self.controller.write_bytes(original)
            sandbox.run(["git", "add", self.controller])

    def cleanup(self, image):
        sandbox = self.sandbox
        exchange = sandbox.root / "compose-cleanup"
        exchange.mkdir()
        environment = sandbox.environment
        sandbox.environment = environment | {
            "PHPWARDEN_ROOT": str(sandbox.project),
            "PHPWARDEN_CWD": str(sandbox.project),
            "PHPWARDEN_WRAPPER": str(self.wrapper),
            "PHPWARDEN_IMAGE": image,
            "PHPWARDEN_UID": str(os.getuid()),
            "PHPWARDEN_GID": str(os.getgid()),
            "PHPWARDEN_GIT_COMMON": str(sandbox.project / ".git"),
            "PHPWARDEN_CACHE": str(sandbox.root / "cache/phpwarden/compose"),
            "PHPWARDEN_EXCHANGE": str(exchange),
        }
        try:
            sandbox.run(
                [
                    "docker",
                    "compose",
                    "--env-file",
                    "/dev/null",
                    "--project-name",
                    self.quality_project,
                    "-f",
                    sandbox.project / ".phpwarden.compose.yaml",
                    "down",
                    "--remove-orphans",
                ]
            )
            assert not sandbox.run(
                [
                    "docker",
                    "ps",
                    "-aq",
                    "--filter",
                    "label=com.docker.compose.project=" + self.quality_project,
                ]
            )
        finally:
            sandbox.environment = environment

    def run(self):
        sandbox = self.sandbox
        sandbox.stage("checking Compose wrapper against the running Drupal application")
        image = self.image()
        for name, source in (
            ("phpwarden", "phpwarden"),
            (".phpwarden.compose.yaml", "phpwarden.compose.yaml"),
        ):
            assert not (sandbox.project / name).exists()
            shutil.copy2(sandbox.repository / "examples/compose" / source, sandbox.project / name)
        (sandbox.project / ".phpwarden-image").write_text(image + "\n")
        foreign = sandbox.project / ".agents/skills/project-preserved/SKILL.md"
        foreign.parent.mkdir(parents=True)
        foreign.write_text(
            "---\nname: project-preserved\ndescription: Fixture skill\n---\nKeep me.\n"
        )
        (sandbox.project / ".env").write_text(
            "COMPOSE_PROJECT_NAME=foreign-application\nPHPWARDEN_IMAGE=untrusted:latest\n"
        )
        application_file = sandbox.root / "compose.json"
        application_config = application_file.read_bytes()
        (sandbox.project / "compose.yaml").write_bytes(application_config)
        inputs = (
            "composer.json",
            "composer.lock",
            "vendor/composer/installed.json",
            "vendor/autoload.php",
            "web/index.php",
            ".env",
            "compose.yaml",
            ".agents/skills/project-preserved/SKILL.md",
        )
        preserved = self.snapshot(inputs)
        sandbox.compose("up", "-d", "--no-build", "--pull", "never", "php")
        before = self.application()
        response, database = sandbox.http("off"), self.database()
        configuration = sandbox.project / ".phpwarden.yaml"
        original_config = configuration.read_bytes() if configuration.exists() else None
        sandbox.binary = self.wrapper
        try:
            dirty = self.controller.read_text().replace("$size = 0;", "$size=0;", 1)
            assert dirty != self.controller.read_text()
            self.controller.write_text(dirty)
            sandbox.project_checks("compose", completion=False, require_fix=True)
            if original_config is not None:
                assert configuration.read_bytes() == original_config
            preparation = json.loads(sandbox.run([self.host, "prepare", "--format=json"]))
            assert preparation["ready"]
            metadata = json.loads(sandbox.cli("--build-info"))
            assert metadata["distribution"] == "container"
            assert (
                metadata["commit"]
                == sandbox.run(["git", "rev-parse", "HEAD"], cwd=sandbox.repository).strip()
            )
            hook = (sandbox.project / ".git/hooks/pre-commit").read_text()
            assert "\"$project_root\"/'phpwarden'" in hook, hook
            self.failures()
            self.staged_mismatch()
            status = sandbox.git_status()
            report = json.loads(
                sandbox.cli("check", "--mode=full", "--only=syntax,debug", "--format=json")
            )
            assert report["complete"] and status == sandbox.git_status()
            assert preserved == self.snapshot(inputs)
            assert application_config == application_file.read_bytes()
            assert before == self.application(), "Compose wrapper changed the application container"
            assert response == sandbox.http("off") and database == self.database()
            for path in (
                configuration,
                self.controller,
                self.wrapper,
                sandbox.project / ".git/hooks/pre-commit",
            ):
                assert (path.stat().st_uid, path.stat().st_gid) == (os.getuid(), os.getgid()), path
            sandbox.checkpoint("Verified Compose installation")
            sandbox.summary.setdefault("installations", {})["compose"] = {
                "installed": True,
                "reinitialized": True,
                "check_complete": True,
                "fix_applied": True,
                "precommit_verified": True,
                "staged_mismatch_rejected": True,
                "runtime_mismatch_rejected": True,
                "foreign_hook_preserved": True,
                "project_inputs_preserved": True,
                "application_container_preserved": True,
                "database_config_preserved": True,
                "ownership_preserved": True,
                "worker_image": image,
                "host_only_commands_rejected": True,
            }
        finally:
            sandbox.binary = self.host
            self.cleanup(image)
            assert before == self.application(), "Quality cleanup changed the application container"
