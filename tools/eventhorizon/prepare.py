import argparse
import hashlib
import io
import json
import shutil
import tarfile
import tempfile
import urllib.request
from pathlib import Path


def unpack(source, archive_path, output, prefix, licenses):
    if archive_path:
        data = Path(archive_path).read_bytes()
    else:
        with urllib.request.urlopen(source["url"], timeout=60) as response:
            data = response.read()
    if hashlib.sha256(data).hexdigest() != source["sha256"]:
        raise ValueError("Source archive checksum mismatch")
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        for member in archive.getmembers():
            parts = Path(member.name).parts
            if not parts or not member.isfile():
                continue
            relative = Path(*parts[1:])
            if relative.as_posix() in licenses:
                target = output / licenses[relative.as_posix()]
            elif relative.is_relative_to(prefix) and relative.suffix == ".py":
                target = output / relative.relative_to(prefix)
            else:
                continue
            if (
                relative.is_absolute()
                or ".." in relative.parts
                or not target.is_relative_to(output)
            ):
                raise ValueError("Unsafe archive member")
            target.parent.mkdir(parents=True, exist_ok=True)
            stream = archive.extractfile(member)
            if stream is None:
                raise ValueError("Missing archive member")
            target.write_bytes(stream.read())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-archive")
    parser.add_argument("--yaml-archive")
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent
    sources = json.loads((directory / "sources.json").read_text())
    with tempfile.TemporaryDirectory(prefix=".prepare-", dir=directory) as temporary:
        output = Path(temporary) / "vendor"
        unpack(
            sources["eventhorizon"],
            args.source_archive,
            output,
            Path("src"),
            {"LICENSE": "EVENTHORIZON-LICENSE"},
        )
        unpack(
            sources["pyyaml"], args.yaml_archive, output, Path("lib"), {"LICENSE": "PYYAML-LICENSE"}
        )
        for required in [
            "eventhorizon/scanner/caching_analyzer.py",
            "yaml/__init__.py",
            "EVENTHORIZON-LICENSE",
            "PYYAML-LICENSE",
        ]:
            if not (output / required).is_file():
                raise ValueError(f"Missing source: {required}")
        (output / "source.json").write_text(
            json.dumps(
                {
                    "version": sources["version"],
                    "revision": sources["revision"],
                    "pyyaml": sources["pyyaml"]["version"],
                }
            )
            + "\n"
        )
        if (directory / "vendor").exists():
            shutil.rmtree(directory / "vendor")
        output.rename(directory / "vendor")


if __name__ == "__main__":
    main()
