import argparse
import hashlib
import json
import os
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "internal/legal"


def go(*args):
    return subprocess.run(
        ["go", *args],
        cwd=ROOT,
        env=os.environ | {"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"},
        capture_output=True,
        text=True,
        check=True,
    ).stdout


def json_stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def notices(path):
    files = sorted(
        file
        for file in path.rglob("*")
        if file.is_file()
        and file.name.upper().startswith(("LICENSE", "COPYING", "NOTICE", "PATENTS"))
        and file.suffix not in (".go", ".py", ".html")
    )
    if not files:
        raise ValueError(f"No license material found for {path.name}")
    return files


def generate():
    subprocess.run(["go", "mod", "verify"], cwd=ROOT, check=True, capture_output=True)
    packages = list(json_stream(go("list", "-mod=readonly", "-deps", "-json", "./cmd/phpwarden")))
    modules = {}
    for package in packages:
        module = package.get("Module")
        if not module or module.get("Main"):
            continue
        if module.get("Replace") or not module.get("Version") or not module.get("Sum"):
            raise ValueError("Distribution dependencies must use checksum-pinned modules")
        modules[module["Path"]] = module
    toolchain = json.loads(go("env", "-json", "GOROOT", "GOVERSION"))
    entries = [("Go", toolchain["GOVERSION"], Path(toolchain["GOROOT"]), None)]
    entries += [
        (name, module["Version"], Path(module["Dir"]), module["Sum"])
        for name, module in sorted(modules.items())
    ]
    inventory = {"version": 1, "target": "linux/amd64", "cgo": False, "components": []}
    text = [
        "PHPWarden third-party notices\n",
        "Built CLI dependencies for Linux amd64, CGO disabled.\n",
        "Module notices include upstream license material and bundled subcomponents.\n",
        "SPX is an optional runtime extension; its source and license remain in its image.\n\n",
    ]
    for name, version, directory, checksum in entries:
        files = [directory / "LICENSE"] if name == "Go" else notices(directory)
        record = {"module": name, "version": version, "license_files": []}
        if checksum:
            record["go_sum"] = checksum
        for file in files:
            data = file.read_bytes()
            relative = file.relative_to(directory).as_posix()
            record["license_files"].append(
                {"path": relative, "sha256": hashlib.sha256(data).hexdigest()}
            )
            text += [
                f"===== {name} {version}: {relative} =====\n",
                data.decode("utf-8").replace("\r\n", "\n"),
                "\n\n",
            ]
        inventory["components"].append(record)
    return {
        "THIRD_PARTY_NOTICES.txt": "".join(text).encode(),
        "dependencies.json": (json.dumps(inventory, indent=2) + "\n").encode(),
    }


def main():
    parser = argparse.ArgumentParser(description="Check or regenerate pinned CLI license material")
    parser.add_argument("command", choices=("check", "generate"))
    args = parser.parse_args()
    outputs = generate()
    for name, content in outputs.items():
        path = OUTPUT / name
        if args.command == "generate":
            path.write_bytes(content)
        elif not path.exists() or path.read_bytes() != content:
            raise SystemExit(
                "CLI license inventory changed; run python3 tools/licenses.py generate"
            )
    print(f"CLI license inventory {args.command}: {len(outputs)} files verified")


if __name__ == "__main__":
    main()
