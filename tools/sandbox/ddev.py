import hashlib
import json
from pathlib import Path


class DdevRuntime:
    def __init__(self, sandbox):
        self.sandbox = sandbox
        sandbox.container_root = "/var/www/html"
        sandbox.ini = "/etc/php/8.3/cli/conf.d/99-phpwarden-spx.ini"
        sandbox.http_url = "http://127.0.0.1/phpwarden-probe"
        sandbox.environment |= {
            "DDEV_XDG_CONFIG_HOME": str(sandbox.root / "ddev-global"),
            "DDEV_NONINTERACTIVE": "true",
            "CAROOT": str(sandbox.root / "certificates"),
        }
        self.started = False

    def ddev(self, *arguments):
        return self.sandbox.run(["ddev", "--skip-hooks", *arguments], timeout=600)

    def globals(self):
        sandbox = self.sandbox
        containers = sandbox.run(
            [
                "docker",
                "ps",
                "-aq",
                "--filter",
                "name=^/ddev-router$",
                "--filter",
                "name=^/ddev-ssh-agent$",
            ]
        ).split()
        result = {"containers": {}, "files": {}}
        for container in containers:
            template = (
                '{"image":{{json .Image}},"started":{{json .State.StartedAt}},'
                '"running":{{json .State.Running}},"restarts":{{json .RestartCount}}}'
            )
            result["containers"][container] = json.loads(
                sandbox.run(["docker", "inspect", "--format", template, container])
            )
        for path in (
            Path.home() / ".ddev/global_config.yaml",
            Path.home() / ".config/ddev/global_config.yaml",
            Path("/etc/hosts"),
        ):
            result["files"][str(path)] = (
                hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None
            )
        return result

    def install(self):
        sandbox = self.sandbox
        sandbox.stage("preparing isolated DDEV configuration and SQLite application")
        self.before_globals = self.globals()
        self.ddev(
            "config",
            "global",
            "--omit-containers=ddev-router,ddev-ssh-agent",
            "--instrumentation-opt-in=false",
            "--performance-mode=none",
            "--no-tui",
        )
        version = json.loads(self.ddev("version", "--json-output"))["raw"]["DDEV version"]
        sandbox.summary["ddev"] = {"version": version}
        directory = sandbox.project / ".ddev"
        directory.mkdir()
        (directory / "config.yaml").write_text(
            json.dumps(
                {
                    "name": sandbox.name,
                    "type": "drupal" + sandbox.major,
                    "docroot": "web",
                    "php_version": "8.3",
                    "webserver_type": "nginx-fpm",
                    "omit_containers": ["db", "ddev-ssh-agent"],
                    "performance_mode": "none",
                    "disable_settings_management": True,
                    "use_dns_when_possible": True,
                    "nodejs_version": "22.22.3",
                    "disable_upload_dirs_warning": True,
                },
                indent=2,
            )
        )
        cache = sandbox.root / "ddev-cache"
        cache.mkdir()
        (directory / "docker-compose.sandbox.yaml").write_text(
            json.dumps(
                {
                    "services": {
                        "web": {
                            "volumes": [
                                f"{sandbox.root / 'data'}:/data",
                                f"{cache}:/mnt/ddev-global-cache",
                            ]
                        }
                    },
                },
                indent=2,
            )
        )
        self.start()
        sandbox.container = self.current_container()
        assert sandbox.exec(["-r", 'echo extension_loaded("spx")?"yes":"no";']) == "no"
        actual = json.loads(
            sandbox.exec(
                [
                    "-r",
                    'echo json_encode(["php"=>PHP_VERSION,'
                    '"sqlite"=>SQLite3::version()["versionString"]]);',
                ]
            )
        )
        assert actual["php"].startswith("8.3.")
        sandbox.summary["ddev"].update(actual)
        sandbox.drush("cache:rebuild")
        sandbox.http("off")
        sandbox.checkpoint("Installed isolated DDEV fixture")
        self.inputs = {
            name: (sandbox.project / name).read_bytes()
            for name in (
                ".ddev/config.yaml",
                ".ddev/docker-compose.sandbox.yaml",
                "web/sites/default/settings.php",
            )
        }
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
        self.started = True
        self.ddev("start")

    def current_container(self):
        container = "ddev-" + self.sandbox.name + "-web"
        row = json.loads(self.sandbox.run(["docker", "inspect", container]))[0]
        assert row["State"]["Running"] and row["Config"]["Labels"]["com.ddev.approot"] == str(
            self.sandbox.project
        )
        return row["Id"]

    def setup_arguments(self):
        return ["--environment=ddev", "--php=8.3", "--container-root=/var/www/html"]

    def preserved(self):
        sandbox = self.sandbox
        assert all(
            (sandbox.project / name).read_bytes() == content
            for name, content in self.inputs.items()
        )
        assert self.db_config == self.database()
        assert self.before_globals == self.globals(), "DDEV changed existing global state"
        sandbox.summary["ddev"].update(
            {
                "inputs_preserved": True,
                "database_config_preserved": True,
                "global_state_preserved": True,
            }
        )

    def cleanup(self):
        sandbox = self.sandbox
        if self.started:
            try:
                if not sandbox.summary["passed"]:
                    sandbox.run(
                        ["docker", "logs", "--tail=100", "ddev-" + sandbox.name + "-web"],
                        expected=(0, 1),
                    )
            finally:
                self.ddev("stop", "--unlist", sandbox.name)
            assert not sandbox.run(
                ["docker", "ps", "-aq", "--filter", "label=com.ddev.site-name=" + sandbox.name]
            )
        if hasattr(self, "before_globals"):
            assert self.before_globals == self.globals(), "DDEV cleanup changed global state"
