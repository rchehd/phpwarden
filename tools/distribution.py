import argparse
import base64
import hashlib
import json
import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

RUNTIMES = ("8.1", "8.2", "8.3", "8.4")
REPOSITORY = "ghcr.io/rchehd/phpwarden"


def published_digests(repository, run=subprocess.run):
    if repository != REPOSITORY:
        raise ValueError("Distribution manifests require the official GHCR repository")
    result = {}
    for runtime in RUNTIMES:
        execution = run(
            ["docker", "image", "inspect", f"phpwarden:static-php{runtime}-dev"],
            check=True,
            capture_output=True,
            text=True,
        )
        images = json.loads(execution.stdout)
        if len(images) != 1:
            raise ValueError("Expected exactly one tested local image")
        matches = {
            value
            for value in images[0].get("RepoDigests", [])
            if re.fullmatch(re.escape(repository) + r"@sha256:[0-9a-f]{64}", value)
        }
        if len(matches) != 1:
            raise ValueError(f"PHP {runtime} has no unambiguous published digest")
        result[runtime] = matches.pop()
    return result


def build_manifest(version, repository, output):
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="phpwarden-publish-") as directory:
        images = Path(directory) / "images.json"
        images.write_text(json.dumps(published_digests(repository)))
        subprocess.run(
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
                str(output),
            ],
            check=True,
        )


def package(version, commit, manifest, output):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", version):
        raise ValueError("Expected an exact release or development snapshot version")
    if not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("Expected an exact Git commit")
    output.mkdir(parents=True, exist_ok=True)
    binary = output / "phpwarden"
    distribution = "standalone" if manifest else "source"
    flags = (
        f"-s -w -X github.com/rchehd/phpwarden.Version={version} "
        f"-X github.com/rchehd/phpwarden.Commit={commit} "
        f"-X github.com/rchehd/phpwarden.Distribution={distribution}"
    )
    if manifest:
        encoded = base64.b64encode(manifest.read_bytes()).decode("ascii")
        flags += f" -X github.com/rchehd/phpwarden.ManifestBase64={encoded}"
    subprocess.run(
        [
            "go",
            "build",
            "-trimpath",
            "-ldflags",
            flags,
            "-o",
            str(binary),
            "./cmd/phpwarden",
        ],
        env=os.environ | {"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"},
        check=True,
    )
    subprocess.run([str(binary.resolve()), "--version"], check=True)
    arguments = [
        "go",
        "run",
        "./cmd/phpwarden-dist",
        "package",
        "--binary",
        str(binary),
        "--output",
        str(output),
    ]
    if manifest:
        arguments.extend(["--manifest", str(manifest)])
    subprocess.run(arguments, check=True)
    installer = output / "install.sh"
    shutil.copyfile("install.sh", installer)
    with (output / "SHA256SUMS").open("a") as checksums:
        checksums.write(f"{hashlib.sha256(installer.read_bytes()).hexdigest()}  install.sh\n")


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    manifest = commands.add_parser("manifest")
    manifest.add_argument("--version", required=True)
    manifest.add_argument("--repository", required=True)
    manifest.add_argument("--output", type=Path, required=True)
    archive = commands.add_parser("package")
    archive.add_argument("--version", required=True)
    archive.add_argument("--commit", required=True)
    archive.add_argument("--manifest", type=Path)
    archive.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "manifest":
        build_manifest(args.version, args.repository, args.output)
    else:
        package(args.version, args.commit, args.manifest, args.output)


if __name__ == "__main__":
    main()
