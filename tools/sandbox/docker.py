import hashlib
import json
from pathlib import Path

from tools.sandbox.assertions import container_snapshot


class DockerRuntime:
    def __init__(self, sandbox):
        self.sandbox = sandbox
        self.name = sandbox.name + "-php"
        self.original_name = self.name + "-original"
        self.sibling = sandbox.name + "-sibling"
        self.owner = "io.phpwarden.sandbox=" + sandbox.name
        self.original_id = None
        self.active = False
        self.restored = False

    def inspect(self, container):
        return json.loads(self.sandbox.run(["docker", "inspect", container]))[0]

    def spec(self, container, started=False):
        return container_snapshot(self.inspect(container), started=started)

    def run_arguments(self, image, extra=()):
        sandbox = self.sandbox
        return [
            "docker",
            "run",
            "--detach",
            "--pull=never",
            "--name",
            self.name,
            "--label",
            self.owner,
            "--network=none",
            "--user",
            sandbox.user,
            "--read-only",
            "--cap-drop=ALL",
            "--security-opt=no-new-privileges:true",
            "--init",
            "--tmpfs",
            "/tmp:rw,nosuid,nodev,size=256m",
            "--mount",
            f"type=bind,src={sandbox.project},dst=/project,readonly",
            "--mount",
            f"type=bind,src={sandbox.root / 'data'},dst=/data",
            "--mount",
            f"type=bind,src={sandbox.project / 'web/sites/default/files'},"
            "dst=/project/web/sites/default/files",
            "--workdir",
            "/project/web",
            "--entrypoint",
            "php",
            *extra,
            image,
            "-S",
            "127.0.0.1:8080",
            "-t",
            "/project/web",
            "/project/web/index.php",
        ]

    def install(self):
        sandbox = self.sandbox
        sandbox.stage("starting the known bare Docker launch and an unrelated fixture sibling")
        self.base_image = sandbox.run(
            ["docker", "image", "inspect", sandbox.image, "--format", "{{.Id}}"]
        ).strip()
        self.original_id = sandbox.run(self.run_arguments(self.base_image)).strip()
        self.write_launch_evidence()
        sandbox.run(
            [
                "docker",
                "run",
                "--detach",
                "--pull=never",
                "--name",
                self.sibling,
                "--label",
                self.owner,
                "--network=none",
                "--read-only",
                "--user",
                sandbox.user,
                "--cap-drop=ALL",
                "--security-opt=no-new-privileges:true",
                "--entrypoint",
                "php",
                self.base_image,
                "-r",
                "while (true) { sleep(60); }",
            ]
        )
        self.sibling_before = self.spec(self.sibling, started=True)
        sandbox.container = self.current_container()
        sandbox.drush("cache:rebuild")
        sandbox.http("off")
        assert sandbox.exec(["-r", 'echo extension_loaded("spx")?"yes":"no";']) == "no"
        self.settings = (sandbox.project / "web/sites/default/settings.php").read_bytes()
        self.db_config = self.database()

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

    def start(self):
        assert self.original_id is not None
        assert self.inspect(self.name)["State"]["Running"]

    def current_container(self):
        row = self.inspect(self.name)
        assert row["State"]["Running"]
        assert row["Config"]["Labels"]["io.phpwarden.sandbox"] == self.sandbox.name
        return row["Id"]

    def setup_arguments(self):
        if not self.active:
            self.before_setup = self.spec(self.name, started=True)
        return [
            "--environment=docker",
            "--container",
            self.name,
            "--php=8.3",
            "--container-root=/project",
        ]

    def launch_arguments(self, setup):
        arguments = setup["launch_arguments"]
        assert len(arguments) == 5 and arguments[0] == arguments[2] == "--mount"
        assert arguments[-1] == setup["image"]
        directory = self.sandbox.root / "state"
        reports = Path(setup["reports"])
        assert reports.is_relative_to(directory) and reports.name == "reports"
        expected = [
            f"type=bind,src={reports},dst=/var/lib/phpwarden-spx",
            f"type=bind,src={reports.parent / 'spx.ini'},"
            "dst=/usr/local/etc/php/conf.d/99-phpwarden-spx.ini,readonly",
        ]
        assert arguments[1:4:2] == expected
        return arguments[:-1]

    def activate(self, setup):
        sandbox = self.sandbox
        sandbox.stage("manually activating returned SPX arguments while retaining the original")
        assert setup["state"] == "prepared" and not setup["loaded"]
        assert setup["runtime_checked"]
        assert self.before_setup == self.spec(self.name, started=True)
        assert self.sibling_before == self.spec(self.sibling, started=True)
        self.original_spec = self.spec(self.name)
        extra = self.launch_arguments(setup)
        rejected = sandbox.lifecycle("teardown", "--apply", expected=(3,))
        assert rejected["state"] == "error" and rejected["diagnostics"]
        assert self.before_setup == self.spec(self.name, started=True)
        sandbox.run(["docker", "stop", self.original_id])
        sandbox.run(["docker", "rename", self.original_id, self.original_name])
        self.profiled_id = sandbox.run(self.run_arguments(setup["image"], extra)).strip()
        self.active = True
        sandbox.container = self.current_container()
        status = sandbox.lifecycle("status")
        assert status["loaded"] and status["state"] == "active" and status["runtime_checked"]
        self.profiled_spec = self.spec(self.name, started=True)
        repeated = sandbox.lifecycle(
            "setup",
            *self.setup_arguments(),
            "--http",
            "--spx-archive",
            sandbox.root / "spx.tar.gz",
            "--apply",
        )
        assert repeated["loaded"] and self.profiled_spec == self.spec(self.name, started=True)
        assert self.sibling_before == self.spec(self.sibling, started=True)
        original = self.inspect(self.original_id)
        assert original["Image"] == self.base_image and not original["State"]["Running"]
        sandbox.summary["docker"] = {
            "setup_kept_original_running": True,
            "manual_activation_verified": True,
            "apply_teardown_rejected": True,
            "repeated_setup_kept_container": True,
            "base_image": self.base_image,
            "original_container": self.original_id,
        }
        return status

    def restore(self):
        sandbox = self.sandbox
        sandbox.stage("restoring the retained bare Docker container before removing adapters")
        assert self.active and self.current_container() == self.profiled_id
        sandbox.run(["docker", "rm", "--force", self.profiled_id])
        sandbox.run(["docker", "rename", self.original_id, self.name])
        sandbox.run(["docker", "start", self.original_id])
        self.active = False
        sandbox.container = self.current_container()
        assert self.original_spec == self.spec(self.name)
        assert sandbox.exec(["-r", 'echo extension_loaded("spx")?"yes":"no";']) == "no"
        status = sandbox.lifecycle("status")
        assert not status["loaded"] and status["runtime_checked"]
        sandbox.http("off")
        self.restored = True
        sandbox.summary["docker"]["original_container_restored"] = True

    def preserved(self):
        sandbox = self.sandbox
        assert not self.active and self.current_container() == self.original_id
        assert self.original_spec == self.spec(self.name)
        assert self.sibling_before == self.spec(self.sibling, started=True)
        assert self.settings == (sandbox.project / "web/sites/default/settings.php").read_bytes()
        assert self.db_config == self.database()
        self.write_launch_evidence()
        sandbox.summary["docker"].update(
            {
                "launch_spec_preserved": True,
                "sibling_preserved": True,
                "database_config_preserved": True,
            }
        )

    def write_launch_evidence(self):
        sandbox = self.sandbox
        arguments = self.run_arguments(self.base_image)
        encoded = json.dumps(arguments, separators=(",", ":")).encode()
        (sandbox.root / "docker-launch.json").write_text(
            json.dumps(
                {
                    "arguments": arguments,
                    "sha256": hashlib.sha256(encoded).hexdigest(),
                    "original_container": self.original_id,
                    "restored": self.restored,
                },
                indent=2,
            )
        )

    def cleanup(self):
        sandbox = self.sandbox
        containers = sandbox.run(["docker", "ps", "-aq", "--filter", "label=" + self.owner]).split()
        for container in containers:
            row = self.inspect(container)
            assert row["Name"].removeprefix("/") in (self.name, self.original_name, self.sibling)
        if containers:
            sandbox.run(["docker", "rm", "--force", *containers])
        assert not sandbox.run(["docker", "ps", "-aq", "--filter", "label=" + self.owner])
