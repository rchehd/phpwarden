import argparse
import hashlib
import io
import json
import platform
import shutil
import tarfile
import tempfile
import urllib.request
import zipfile
from pathlib import Path


def download(source, filename):
    if filename:
        data = Path(filename).read_bytes()
    else:
        with urllib.request.urlopen(source["url"], timeout=60) as response:
            data = response.read()
    if hashlib.sha256(data).hexdigest() != source["sha256"]:
        raise ValueError("Semgrep source checksum mismatch")
    return data


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--platform", choices=["manylinux_2_34_x86_64", "musllinux_1_2_x86_64"])
    parser.add_argument("--wheel-archive")
    parser.add_argument("--source-archive")
    args = parser.parse_args()
    if platform.system() != "Linux" or platform.machine() not in ("x86_64", "amd64"):
        raise ValueError("The development Semgrep bundle supports Linux amd64")
    selected = args.platform or (
        "manylinux_2_34_x86_64" if platform.libc_ver()[0] == "glibc" else "musllinux_1_2_x86_64"
    )
    directory = Path(__file__).resolve().parent
    sources = json.loads((directory / "sources.json").read_text())
    wheel = download(sources["wheels"][selected], args.wheel_archive)
    source = download(sources["source"], args.source_archive)
    with tempfile.TemporaryDirectory(prefix=".prepare-", dir=directory) as temporary:
        output = Path(temporary) / "vendor"
        output.mkdir()
        with zipfile.ZipFile(io.BytesIO(wheel)) as archive:
            prefix = f"semgrep-{sources['version']}.data/purelib/semgrep/bin/"
            for name in archive.namelist():
                if not name.startswith(prefix) or name.endswith("/"):
                    continue
                relative = Path(name[len(prefix) :])
                if relative.name == "__init__.py":
                    continue
                if relative.is_absolute() or ".." in relative.parts:
                    raise ValueError("Unsafe wheel member")
                target = output / "bin" / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(archive.read(name))
        binary = output / "bin/semgrep-core"
        binary.chmod(0o755)
        with tarfile.open(fileobj=io.BytesIO(source), mode="r:gz") as archive:
            for member in archive.getmembers():
                relative = Path(*Path(member.name).parts[1:])
                if not member.isfile() or not relative.name.lower().startswith(
                    ("license", "copying", "notice", "authors")
                ):
                    continue
                if relative.is_absolute() or ".." in relative.parts:
                    raise ValueError("Unsafe source member")
                target = output / "notices" / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                stream = archive.extractfile(member)
                if stream is None:
                    raise ValueError("Missing source notice")
                target.write_bytes(stream.read())
        if not (output / "notices/LICENSE").is_file():
            raise ValueError("Missing upstream LGPL license")
        (output / "semgrep-source.tar.gz").write_bytes(source)
        (output / "sources.json").write_text(json.dumps(sources, indent=2) + "\n")
        if (directory / "vendor").exists():
            shutil.rmtree(directory / "vendor")
        output.rename(directory / "vendor")


if __name__ == "__main__":
    main()
