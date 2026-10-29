import argparse
import hashlib
import json
import os
import subprocess
import time
from pathlib import Path

VERSION = "6.20.3"
SHA256 = "bda19a8cbe8914a7eab079c4a33d25e69ca20856e5e0c4ae5c9dcab1d8ecc828"

FRAMEWORK = """<?php
namespace Symfony\\Component\\HttpFoundation;
class InputBag {
    public function get(string $key): string { return ''; }
}
namespace Drupal\\Core\\Render;
class Markup {
    public static function create(string $string): self { return new self(); }
}
namespace Drupal\\Component\\Utility;
class Html {
    public static function escape(string $text): string { return $text; }
}
"""

STUBS = """<?php
namespace Symfony\\Component\\HttpFoundation;
class InputBag {
    /** @psalm-taint-source input */
    public function get(string $key): string {}
}
namespace Drupal\\Core\\Render;
class Markup {
    /** @psalm-taint-sink html $string */
    public static function create(string $string): self {}
}
namespace Drupal\\Component\\Utility;
class Html {
    /** @psalm-taint-escape html */
    public static function escape(string $text): string {}
}
"""

RELAY = """<?php
function relay(string $value): string { return $value; }
function requestName(): string { return $_GET['name']; }
"""
REQUEST = "$request = new \\Symfony\\Component\\HttpFoundation\\InputBag();\n"
VALUE = "relay($request->get('name'))"
SINK = "\\Drupal\\Core\\Render\\Markup::create"
ESCAPE = "\\Drupal\\Component\\Utility\\Html::escape"
CASES = [
    ("direct-html", "echo $_GET['name'];", False, ["TaintedHtml", "TaintedTextWithQuotes"]),
    (
        "cross-file-html",
        "echo relay(requestName());",
        False,
        ["TaintedHtml", "TaintedTextWithQuotes"],
    ),
    (
        "escaped-html",
        "echo htmlspecialchars(relay(requestName()), ENT_QUOTES, 'UTF-8');",
        False,
        [],
    ),
    ("framework-unmodeled", REQUEST + f"{SINK}({VALUE});", False, []),
    ("framework-modeled", REQUEST + f"{SINK}({VALUE});", True, ["TaintedHtml"]),
    ("framework-escaped", REQUEST + f"{SINK}({ESCAPE}({VALUE}));", True, []),
]


def docker(args, project=None, *, composer=False):
    command = [
        "docker",
        "run",
        "--pull",
        "never",
        "--rm",
        "--network",
        "none",
        "--read-only",
        "--user",
        f"{os.getuid()}:{os.getgid()}",
        "--cap-drop",
        "ALL",
        "--security-opt",
        "no-new-privileges",
        "--pids-limit",
        "128",
        "--memory",
        "1g",
        "--tmpfs",
        "/tmp:rw,nosuid,size=512m,mode=1777",
        "--mount",
        f"type=bind,src={args.phar},dst=/opt/psalm.phar,readonly",
    ]
    if project is not None:
        access = "" if composer else ",readonly"
        command += ["--mount", f"type=bind,src={project},dst=/workspace{access}"]
    return (
        command
        + [
            "--workdir",
            "/workspace",
            "--entrypoint",
            "composer" if composer else args.php,
            args.image,
        ]
        + ([] if composer else ["/opt/psalm.phar"])
    )


def main():
    parser = argparse.ArgumentParser(description="Isolated Psalm taint feasibility spike")
    parser.add_argument("--phar", type=Path, required=True)
    parser.add_argument(
        "--image", required=True, help="Existing local image containing supported PHP and Composer"
    )
    parser.add_argument("--php", default="php", help="PHP executable inside the image")
    parser.add_argument("--output", type=Path, required=True, help="New artifact directory")
    args = parser.parse_args()
    args.phar = args.phar.resolve(strict=True)
    if hashlib.sha256(args.phar.read_bytes()).hexdigest() != SHA256:
        parser.error("PHAR checksum does not match the pinned release")
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False)
    version = subprocess.run(
        docker(args) + ["--version"], capture_output=True, text=True, timeout=30, check=True
    ).stdout.strip()
    if not version.startswith(f"Psalm {VERSION}@"):
        parser.error(f"Unexpected tool identity: {version}")
    summary = {"version": version, "sha256": SHA256, "image": args.image, "cases": []}
    try:
        for name, entry, modeled, expected in CASES:
            project = args.output / name
            (project / "src").mkdir(parents=True)
            (project / "vendor").mkdir()
            (project / "src/Relay.php").write_text(RELAY)
            (project / "vendor/Framework.php").write_text(FRAMEWORK)
            (project / "src/entry.module").write_text("<?php\n" + entry + "\n")
            (project / "composer.json").write_text(
                json.dumps(
                    {
                        "name": "phpwarden/taint-spike",
                        "autoload": {
                            "files": ["src/Relay.php"],
                            "classmap": ["vendor/Framework.php"],
                        },
                    }
                )
                + "\n"
            )
            autoload = subprocess.run(
                docker(args, project, composer=True)
                + [
                    "--no-cache",
                    "dump-autoload",
                    "--no-scripts",
                    "--no-plugins",
                    "--no-interaction",
                ],
                capture_output=True,
                text=True,
                timeout=30,
            )
            (project / "autoload.log").write_text(autoload.stdout + autoload.stderr)
            if autoload.returncode:
                raise RuntimeError(
                    f"Fixture autoload generation failed: {project / 'autoload.log'}"
                )
            (project / "framework-stubs.php").write_text(STUBS)
            stubs = '<stubs><file name="framework-stubs.php"/></stubs>' if modeled else ""
            (project / "psalm.xml").write_text(
                '<psalm xmlns="https://getpsalm.org/schema/config" errorLevel="1" '
                'cacheDirectory="/tmp/psalm-cache" resolveFromConfigFile="true">'
                '<projectFiles><directory name="src"/></projectFiles>'
                '<fileExtensions><extension name="php"/><extension name="module"/>'
                f"</fileExtensions>{stubs}</psalm>\n"
            )
            before = {
                str(p.relative_to(project)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in project.rglob("*")
                if p.is_file()
            }
            started = time.monotonic()
            result = subprocess.run(
                docker(args, project)
                + [
                    "--taint-analysis",
                    "--no-cache",
                    "--no-diff",
                    "--no-progress",
                    "--threads=1",
                    "--output-format=json",
                ],
                capture_output=True,
                text=True,
                timeout=90,
            )
            (project / "stdout.json").write_text(result.stdout)
            (project / "stderr.log").write_text(result.stderr)
            issues = json.loads(result.stdout)
            actual = sorted(issue["type"] for issue in issues)
            passed = result.returncode == (2 if expected else 0) and actual == expected
            preserved = all(
                hashlib.sha256((project / p).read_bytes()).hexdigest() == digest
                for p, digest in before.items()
            )
            record = {
                "name": name,
                "exit": result.returncode,
                "expected": expected,
                "actual": actual,
                "passed": passed and preserved,
                "source_preserved": preserved,
                "seconds": round(time.monotonic() - started, 2),
            }
            summary["cases"].append(record)
            print(json.dumps(record), flush=True)
    finally:
        (args.output / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    return 0 if all(case["passed"] for case in summary["cases"]) else 1


if __name__ == "__main__":
    raise SystemExit(main())
