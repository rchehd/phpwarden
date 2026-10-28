import hashlib
import json
import os
import re
import secrets
import shlex
import shutil
import socket
import subprocess
import time
import urllib.request
from pathlib import Path

from tools.sandbox.compose import ComposeInstallation

COMPOSER_URL = "https://getcomposer.org/download/2.10.3/composer.phar"
COMPOSER_SHA256 = "7a2d379d5b8ffdaa028580ef26494c36d2feef4b178d3dd1473a4dbc5e17c8d6"
EXTENSIONS = (
    "ctype",
    "curl",
    "dom",
    "fileinfo",
    "filter",
    "gd",
    "iconv",
    "intl",
    "mbstring",
    "openssl",
    "pdo",
    "pdo_sqlite",
    "phar",
    "simplexml",
    "sqlite3",
    "tokenizer",
    "xml",
    "xmlreader",
    "xmlwriter",
    "zlib",
)


def snapshot(paths):
    return {str(path): hashlib.sha256(path.read_bytes()).hexdigest() for path in paths}


def extension_configuration(info, directories):
    builtins = {name.lower() for name in info["modules"]}
    lines = []
    for name in EXTENSIONS:
        if name in builtins:
            continue
        module = next(
            (
                directory / (name + ".so")
                for directory in directories
                if (directory / (name + ".so")).is_file()
            ),
            None,
        )
        if module is None:
            raise RuntimeError(f"Host PHP needs {name}; supply --host-extensions or install it")
        lines.append("extension=" + json.dumps(str(module)))
    return lines


class HostRuntime:
    def __init__(self, sandbox, php, extensions=None):
        self.sandbox = sandbox
        self.php = shutil.which(php)
        if self.php is None:
            raise ValueError("Host sandbox requires a PHP 8.3 executable")
        self.extensions = extensions
        self.process = None
        self.global_configuration = None

    def prepare(self):
        sandbox = self.sandbox
        probe = json.loads(
            sandbox.run(
                [
                    self.php,
                    "-n",
                    "-r",
                    'echo json_encode(["version"=>PHP_VERSION,"modules"=>get_loaded_extensions(),'
                    '"extension_dir"=>ini_get("extension_dir")]);',
                ]
            )
        )
        assert probe["version"].startswith("8.3."), "Host fixtures require PHP 8.3"
        global_info = json.loads(
            sandbox.run(
                [
                    self.php,
                    "-r",
                    'echo json_encode(["ini"=>php_ini_loaded_file(),'
                    '"scan"=>php_ini_scanned_files()]);',
                ]
            )
        )
        paths = [Path(global_info["ini"])] if global_info["ini"] else []
        paths += [
            Path(value.strip()) for value in (global_info["scan"] or "").split(",") if value.strip()
        ]
        self.global_configuration = snapshot(paths)
        directories = [Path(probe["extension_dir"])]
        if self.extensions:
            directories.insert(0, self.extensions.resolve())
        lines = extension_configuration(probe, directories)
        private = sandbox.root / "php"
        private.mkdir()
        for name in ("scan", "tmp", "sessions", "bin", "composer-home", "composer-cache"):
            (private / name).mkdir()
        self.ini = private / "php.ini"
        self.ini.write_text(
            "\n".join(
                [
                    *lines,
                    "memory_limit=512M",
                    "display_errors=Off",
                    "log_errors=On",
                    "error_log=" + json.dumps(str(sandbox.logs / "php-errors.log")),
                    "sys_temp_dir=" + json.dumps(str(private / "tmp")),
                    "session.save_path=" + json.dumps(str(private / "sessions")),
                    "",
                ]
            )
        )
        wrapper = private / "bin/php"
        wrapper.write_text(
            "#!/bin/sh\nexec "
            + shlex.quote(self.php)
            + " -c "
            + shlex.quote(str(self.ini))
            + ' "$@"\n'
        )
        wrapper.chmod(0o755)
        sandbox.environment.update(
            {
                "PATH": str(wrapper.parent) + os.pathsep + sandbox.environment["PATH"],
                "PHP_INI_SCAN_DIR": str(private / "scan"),
                "PHPRC": str(self.ini),
                "COMPOSER_HOME": str(private / "composer-home"),
                "COMPOSER_CACHE_DIR": str(private / "composer-cache"),
            }
        )
        sandbox.environment.pop("PHP_CLI_SERVER_WORKERS", None)
        self.command = [self.php, "-c", str(self.ini)]
        actual = json.loads(
            self.exec(
                [
                    "-r",
                    'echo json_encode(["version"=>PHP_VERSION,"modules"=>get_loaded_extensions(),'
                    '"ini"=>php_ini_loaded_file(),"scan"=>php_ini_scanned_files(),'
                    '"sqlite"=>SQLite3::version()["versionString"]]);',
                ]
            )
        )
        assert actual["ini"] == str(self.ini) and not actual["scan"]
        assert "spx" not in {value.lower() for value in actual["modules"]}
        assert set(EXTENSIONS) <= {value.lower() for value in actual["modules"]}
        sandbox.summary["host"] = {
            "php": actual["version"],
            "sqlite": actual["sqlite"],
            "private_ini": True,
            "spx_loaded": False,
        }
        self.composer_phar = private / "composer.phar"
        with urllib.request.urlopen(COMPOSER_URL, timeout=30) as response:
            data = response.read((4 << 20) + 1)
        assert len(data) <= 4 << 20 and hashlib.sha256(data).hexdigest() == COMPOSER_SHA256
        self.composer_phar.write_bytes(data)

    def composer(self, *arguments, offline=False):
        sandbox = self.sandbox
        previous = sandbox.environment
        sandbox.environment = previous | ({"COMPOSER_DISABLE_NETWORK": "1"} if offline else {})
        try:
            return sandbox.run([*self.command, self.composer_phar, *arguments], timeout=600)
        finally:
            sandbox.environment = previous

    def exec(self, args, profile=False):
        if profile:
            raise ValueError("Host PHP live SPX profiling is unsupported")
        sandbox = self.sandbox
        aliases = {"/project": str(sandbox.project), "/data": str(sandbox.root / "data")}
        mapped = [
            re.sub(
                r"(?<!\w)/(?:project|data)(?=/|$)",
                lambda match: aliases[match.group()],
                str(arg),
            )
            for arg in args
        ]
        return sandbox.run([*self.command, *mapped])

    def install_project(self):
        sandbox = self.sandbox
        sandbox.stage("installing locked Drupal with native PHP and private configuration")
        self.prepare()
        fixture = sandbox.repository / f"tools/sandbox/drupal{sandbox.major}"
        for name in ("composer.json", "composer.lock"):
            shutil.copyfile(fixture / name, sandbox.project / name)
        assert not json.loads((sandbox.project / "composer.json").read_text()).get("scripts")
        self.composer("install", "--no-dev", "--no-interaction", "--prefer-dist")
        shutil.copytree(
            sandbox.repository / "tools/sandbox/fixture",
            sandbox.project / "web/modules/custom/phpwarden_probe",
        )
        (sandbox.project / "web/sites/default/files").mkdir(parents=True, exist_ok=True)
        (sandbox.root / "data").mkdir()
        settings = sandbox.project / "web/sites/default/settings.php"
        settings.write_text(
            "<?php\n$settings['hash_salt'] = '"
            + secrets.token_hex(32)
            + "';\n$settings['trusted_host_patterns'] = "
            + "['^localhost$', '^127\\\\.0\\\\.0\\\\.1$'];\n"
            + "$settings['config_sync_directory'] = "
            + json.dumps(str(sandbox.root / "data/config-sync"))
            + ";\n"
        )
        sandbox.drush(
            "site:install",
            "minimal",
            "--yes",
            "--db-url=sqlite://localhost//data/site.sqlite?module=sqlite",
            "--account-pass=fixture-local-only",
        )
        sandbox.drush("pm:enable", "phpwarden_probe", "--yes")
        (sandbox.root / "compose.json").write_text(
            json.dumps(
                {
                    "services": {"foreign": {"image": "fixture-preserved:unused"}},
                }
            )
        )
        sandbox.finish_install()

    def start(self):
        if self.process:
            assert self.process.poll() is None, "Owned PHP server exited"
            return
        sandbox = self.sandbox
        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            self.port = reservation.getsockname()[1]
        self.server_command = [
            *self.command,
            "-S",
            f"127.0.0.1:{self.port}",
            "-t",
            str(sandbox.project / "web"),
            str(sandbox.project / "web/index.php"),
        ]
        self.server_log = (sandbox.logs / "host-server.log").open("wb")
        self.process = subprocess.Popen(
            self.server_command,
            cwd=sandbox.project / "web",
            env=sandbox.environment,
            stdout=self.server_log,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        sandbox.http_url = f"http://127.0.0.1:{self.port}/phpwarden-probe"
        for _ in range(100):
            if self.process.poll() is not None:
                raise RuntimeError("Native PHP server exited; see host-server.log")
            try:
                with socket.create_connection(("127.0.0.1", self.port), timeout=0.2):
                    return
            except OSError:
                time.sleep(0.1)
        raise RuntimeError("Native PHP server did not start")

    def application(self):
        assert self.process and self.process.poll() is None
        return {
            "pid": self.process.pid,
            "command": self.server_command,
            "ini": snapshot([self.ini]),
        }

    def install(self):
        sandbox = self.sandbox
        sandbox.stage("starting Drupal on loopback with native PHP")
        sandbox.drush("cache:rebuild")
        self.start()
        self.before = self.application()
        self.response = sandbox.http("off")
        self.database = ComposeInstallation(sandbox).database()
        self.settings = snapshot([sandbox.project / "web/sites/default/settings.php"])

    def preserved(self):
        sandbox = self.sandbox
        assert self.before == self.application(), "Quality checks changed the native server"
        assert self.response == sandbox.http("off")
        assert self.database == ComposeInstallation(sandbox).database()
        drush = json.loads(
            sandbox.drush(
                "php:eval",
                "echo json_encode(['drupal'=>\\Drupal::VERSION,"
                "'driver'=>\\Drupal::database()->driver()]);",
            )
        )
        assert drush["drupal"] == self.response["drupal"] and drush["driver"] == "sqlite"
        assert self.settings == snapshot([Path(name) for name in self.settings])
        assert not sandbox.git_status()
        sandbox.summary.update(passed=True, git_clean=True)
        sandbox.summary["host"].update(
            application_preserved=True,
            database_config_preserved=True,
            settings_preserved=True,
            http_verified=True,
            drush_verified=True,
        )
        sandbox.summary["live_profiling"] = {
            "status": "unsupported",
            "profiles": 0,
            "reason": "Host PHP SPX setup is not supported",
        }

    def cleanup(self):
        try:
            if self.process:
                if self.process.poll() is None:
                    self.process.terminate()
                    try:
                        self.process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        self.process.kill()
                        self.process.wait(timeout=10)
                self.sandbox.summary["host"]["server_stopped"] = True
        finally:
            if hasattr(self, "server_log"):
                self.server_log.close()
            if self.global_configuration is not None:
                assert self.global_configuration == snapshot(
                    [Path(name) for name in self.global_configuration]
                ), "Global PHP configuration changed"
                self.sandbox.summary.setdefault("host", {})["global_configuration_preserved"] = True
