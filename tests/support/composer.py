import hashlib
import json
import tempfile
from datetime import UTC, datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from threading import Thread

from tests.support.scenarios import materialize

FIXTURES = Path(__file__).resolve().parents[1] / "fixtures" / "composer-validate"
TOOLS = (
    "composer-audit",
    "composer-validate",
    "debug",
    "drupal-config",
    "phpcs",
    "phpmd",
    "phpstan",
    "syntax",
    "twig-cs-fixer",
    "whitespace",
    "yaml",
)
RULES = ("composer.validate.error", "composer.lock.error", "composer.validate.warning")
COMPOSER_VERSION = "2.10.3"
ADVISORY = {
    "advisoryId": "PKSA-test-0001",
    "packageName": "acme/lib",
    "remoteId": "x",
    "title": "Acme lib - Remote code execution",
    "link": "https://example.test/PKSA-test-0001",
    "cve": "CVE-2026-0001",
    "affectedVersions": "<2.0.0",
    "sources": [{"name": "test", "remoteId": "x"}],
    "reportedAt": "2026-10-01T00:00:00+00:00",
    "composerRepository": "test",
    "severity": "high",
}
AUDIT_MESSAGE = (
    'Composer file "composer.lock": acme/lib 1.0.0: Acme lib - Remote code execution '
    "(CVE-2026-0001) https://example.test/PKSA-test-0001"
)


def sort_key(row):
    return (row[0], row[1] or "", row[2] or 0)


def update_config(scenario, tools):
    filename = scenario.root / ".phpwarden.yaml"
    config = json.loads(filename.read_text())
    config["tools"] = tools(config.get("tools", {}))
    filename.write_text(json.dumps(config))
    return config


def set_validate(scenario, options):
    update_config(scenario, lambda tools: tools | {"composer-validate": options})


def check_composer_validate_fixtures(test, configure, run):
    for directory in sorted(FIXTURES.iterdir()):
        for mode in ("diff", "full", "whole-files", "paths"):
            with (
                test.subTest(scenario=directory.name, mode=mode),
                materialize(directory) as scenario,
            ):
                configure(scenario)
                update_config(
                    scenario,
                    lambda tools: (
                        tools | {"composer-validate": scenario.spec["settings"] | {"enabled": True}}
                    ),
                )
                changed = scenario.spec["changed_files"]
                arguments = {
                    "diff": [],
                    "full": ["--mode", "full"],
                    "whole-files": ["--whole-files"],
                    "paths": changed,
                }[mode]
                result = run(scenario, arguments)
                expected_mode = "diff" if mode == "diff" else "full"
                rows = [row for row in scenario.spec["findings"] if expected_mode in row["modes"]]
                expected = sorted(
                    ((row["checker"], None, row["line"]) for row in rows), key=sort_key
                )
                actual = sorted(
                    ((row["checker"], row["path"], row["line"]) for row in result["findings"]),
                    key=sort_key,
                )
                test.assertEqual(actual, expected, result)
                test.assertEqual(result["exit_code"], 1 if expected else 0, result)
                test.assertTrue(result["complete"], result)
                test.assertEqual(result["diagnostics"], [])
                test.assertEqual([row["id"] for row in result["checks"]], list(TOOLS))
                test.assertEqual(
                    result["scope"]["project_files"],
                    [
                        name
                        for name in ("composer.json", "composer.lock")
                        if mode == "full" or name in changed
                    ],
                    result,
                )
                for row in result["findings"]:
                    test.assertEqual(row["kind"], "global", row)
                    test.assertIn(row["rule"], RULES, row)
                    test.assertTrue(
                        any(
                            row["message"].startswith(f'Composer file "{name}": ')
                            for name in {"composer.json", *changed}
                        ),
                        row,
                    )
                for expectation in rows:
                    test.assertTrue(
                        any(
                            row["rule"] == expectation["rule"]
                            and row["message"].startswith(
                                f'Composer file "{expectation["path"]}": '
                            )
                            and expectation["contains"] in row["message"]
                            for row in result["findings"]
                        ),
                        (expectation, result["findings"]),
                    )


def check_composer_validate_settings(test, configure, run):
    only = ["--only", "composer-validate"]
    with materialize(FIXTURES / "validate-lock-outdated") as scenario:
        configure(scenario)
        update_config(scenario, lambda tools: {name: {"enabled": False} for name in TOOLS})
        test.assertEqual(run(scenario, [])["exit_code"], 0)
        result = run(scenario, only)
        test.assertEqual(result["exit_code"], 1, result)
        test.assertEqual([row["id"] for row in result["checks"]], ["composer-validate"])
        for options, count, exit_code, reason in (
            ({"fail_on": "never"}, 1, 0, None),
            ({"exclude": ["composer.json"]}, 0, 0, "no_files"),
            ({"include": ["web/modules/custom/missing"]}, 0, 0, "no_files"),
        ):
            with test.subTest(options=options):
                set_validate(scenario, options)
                result = run(scenario, only)
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual(len(result["findings"]), count, result)
                test.assertEqual(result["checks"][0]["reason"], reason, result)
    with materialize(FIXTURES / "validate-warning") as scenario:
        configure(scenario)
        update_config(scenario, lambda tools: {name: {"enabled": False} for name in TOOLS})
        for options, exit_code, severity in (({}, 0, "warning"), ({"strict": True}, 1, "error")):
            with test.subTest(options=options):
                set_validate(scenario, options)
                result = run(scenario, only)
                test.assertEqual(result["exit_code"], exit_code, result)
                test.assertEqual([row["severity"] for row in result["findings"]], [severity])


def cache_key(composer_json, composer_lock, no_dev=False):
    data = (
        f"{len(composer_json)}:".encode()
        + composer_json
        + b"\x00"
        + f"{len(composer_lock)}:".encode()
        + composer_lock
        + b"\x00"
        + COMPOSER_VERSION.encode()
        + b"\x00"
        + (b"true" if no_dev else b"false")
    )
    return hashlib.sha256(data).hexdigest()


class AdvisoryServer:
    def __init__(self):
        self.posts = 0
        outer = self

        class Handler(BaseHTTPRequestHandler):
            def reply(self, payload):
                body = json.dumps(payload).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def do_GET(self):
                api = f"http://127.0.0.1:{outer.port}/api/security-advisories"
                self.reply(
                    {
                        "packages": {},
                        "metadata-url": "/p2/%package%.json",
                        "security-advisories": {"metadata": False, "api-url": api},
                    }
                )

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length", 0)))
                outer.posts += 1
                self.reply({"advisories": {"acme/lib": [ADVISORY]}})

            def log_message(self, *arguments):
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.port = self.server.server_address[1]
        Thread(target=self.server.serve_forever, daemon=True).start()

    def stop(self):
        self.server.shutdown()
        self.server.server_close()


def write_project(scenario, port):
    composer = {
        "name": "phpwarden/audit-fixture",
        "type": "project",
        "repositories": [
            {"type": "composer", "url": f"http://127.0.0.1:{port}"},
            {"packagist.org": False},
        ],
        "config": {"secure-http": False},
        "require": {"acme/lib": "1.0.0"},
    }
    lock = {
        "_readme": ["This file is @generated automatically"],
        "content-hash": "0" * 32,
        "packages": [
            {"name": "acme/lib", "version": "1.0.0", "type": "library", "abandoned": "acme/new"}
        ],
        "packages-dev": [],
        "aliases": [],
        "minimum-stability": "stable",
        "stability-flags": {},
        "prefer-stable": False,
        "prefer-lowest": False,
        "platform": {},
        "platform-dev": {},
        "plugin-api-version": "2.9.0",
    }
    (scenario.root / "composer.json").write_text(json.dumps(composer, indent=4) + "\n")
    (scenario.root / "composer.lock").write_text(json.dumps(lock, indent=4) + "\n")


def audit(test, scenario, run, options=None):
    update_config(
        scenario,
        lambda tools: tools | {"composer-audit": {"enabled": True} | (options or {})},
    )
    return run(scenario, ["--only", "composer-audit"])


def check_composer_audit(test, configure, run, *, network, host_cache=False):
    with (
        materialize(FIXTURES / "validate-unchanged") as scenario,
        tempfile.TemporaryDirectory(prefix="phpwarden-advisories-") as cache,
    ):
        configure(scenario)
        scenario.env["XDG_CACHE_HOME"] = cache
        if network:
            check_composer_audit_network(test, scenario, run)
            return
        write_project(scenario, 1)
        assert_unavailable(test, audit(test, scenario, run, {"offline": True}), 0)
        if not host_cache:
            return
        seed_cache(scenario, cache)
        result = audit(test, scenario, run, {"offline": True})
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(result["environment"]["advisories"]["source"], "cache", result)
        test.assertEqual(result["findings"][0]["message"], AUDIT_MESSAGE)


def assert_unavailable(test, result, exit_code):
    test.assertEqual(result["exit_code"], exit_code, result)
    test.assertFalse(result["complete"], result)
    test.assertEqual(result["checks"][0]["status"], "unavailable", result)
    test.assertIn("advisories_unavailable", [row["code"] for row in result["diagnostics"]], result)


def seed_cache(scenario, cache):
    composer_json = (scenario.root / "composer.json").read_bytes()
    composer_lock = (scenario.root / "composer.lock").read_bytes()
    key = cache_key(composer_json, composer_lock)
    directory = Path(cache) / "phpwarden" / "advisories"
    directory.mkdir(parents=True)
    entry = {
        "version": 1,
        "key": key,
        "fetched_at": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "report": {"advisories": {"acme/lib": [ADVISORY]}, "abandoned": {"acme/lib": "acme/new"}},
    }
    (directory / f"{key}.json").write_text(json.dumps(entry))


def check_composer_audit_network(test, scenario, run):
    server = AdvisoryServer()
    try:
        write_project(scenario, server.port)
        result = audit(test, scenario, run)
        test.assertEqual(result["exit_code"], 0, result)
        test.assertTrue(result["complete"], result)
        test.assertEqual(result["diagnostics"], [])
        test.assertEqual(
            [row["rule"] for row in result["findings"]], ["PKSA-test-0001", "composer.abandoned"]
        )
        test.assertEqual(result["findings"][0]["message"], AUDIT_MESSAGE)
        test.assertEqual(result["environment"]["advisories"]["source"], "network", result)
        test.assertEqual(server.posts, 1)
        result = audit(test, scenario, run)
        test.assertEqual(result["environment"]["advisories"]["source"], "cache", result)
        test.assertEqual(server.posts, 1)
        test.assertEqual(audit(test, scenario, run, {"fail_on": "warning"})["exit_code"], 1)
        ignore = {"id": "CVE-2026-0001", "reason": "patched"}
        result = audit(test, scenario, run, {"ignore": [ignore | {"until": "2999-01-01"}]})
        test.assertEqual([row["rule"] for row in result["findings"]], ["composer.abandoned"])
        result = audit(test, scenario, run, {"ignore": [ignore | {"until": "2000-01-01"}]})
        test.assertEqual(result["findings"][0]["rule"], "PKSA-test-0001", result)
        test.assertIn("ignore expired 2000-01-01", result["findings"][0]["message"])
        result = audit(test, scenario, run, {"abandoned": "ignore"})
        test.assertEqual([row["rule"] for row in result["findings"]], ["PKSA-test-0001"])
        test.assertEqual(server.posts, 1)
    finally:
        server.stop()
    result = audit(test, scenario, run, {"max_age_hours": 0})
    test.assertEqual(result["exit_code"], 0, result)
    test.assertEqual(result["environment"]["advisories"]["source"], "cache", result)
    test.assertEqual(result["checks"][0]["status"], "passed", result)
    test.assertIn("advisories_stale", [row["code"] for row in result["diagnostics"]], result)
    expired = {"max_age_hours": 0, "max_stale_hours": 0}
    assert_unavailable(test, audit(test, scenario, run, expired), 0)
    assert_unavailable(test, audit(test, scenario, run, expired | {"required": True}), 3)
