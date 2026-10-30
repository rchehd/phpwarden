import argparse
import hashlib
import json
import shutil
import stat
import tarfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def read(path):
    if not stat.S_ISREG(path.lstat().st_mode):
        raise ValueError(f"Expected a regular file: {path}")
    return path.read_bytes()


def digest(data):
    return hashlib.sha256(data).hexdigest()


def prepare(bundle, sources, output, *, specification=ROOT):
    lock_data = read(bundle / "composer.lock")
    if lock_data != read(specification / "composer.lock"):
        raise ValueError("Psalm bundle does not use the committed lock")
    lock = json.loads(lock_data)
    installed = json.loads(read(bundle / "vendor/composer/installed.json"))["packages"]
    actual = {item["name"]: item for item in installed}
    expected = {item["name"]: item for item in lock["packages"]}
    if (
        len(actual) != len(installed)
        or len(expected) != len(lock["packages"])
        or actual.keys() != expected.keys()
    ):
        raise ValueError("Installed Psalm packages do not match the lock inventory")
    policy = json.loads(read(specification / "sources.json"))
    inventory = []
    copies = {}
    vendor = (bundle / "vendor").resolve(strict=True)
    for name, package in sorted(expected.items()):
        found = actual[name]
        if found["version"] != package["version"] or found.get("source", {}).get(
            "reference"
        ) != package.get("source", {}).get("reference"):
            raise ValueError(f"Installed Psalm package differs from the lock: {name}")
        directory = vendor / name
        if not directory.resolve(strict=True).is_relative_to(vendor):
            raise ValueError(f"Psalm package escapes its vendor directory: {name}")
        notices = {}
        for path in sorted(directory.rglob("*")):
            if path.is_file() and path.name.upper().startswith(
                ("LICENSE", "LICENCE", "COPYING", "NOTICE", "PATENTS")
            ):
                if not path.resolve(strict=True).is_relative_to(directory.resolve(strict=True)):
                    raise ValueError(f"Psalm notice escapes its package: {name}")
                relative = path.relative_to(directory).as_posix()
                data = read(path)
                notices[relative] = digest(data)
                copies[f"notices/{name}/{relative}"] = data
        if not notices or not package.get("license"):
            raise ValueError(f"Psalm package has no license declaration or notice: {name}")
        inventory.append(
            {
                "name": name,
                "version": package["version"],
                "reference": package.get("source", {}).get("reference"),
                "source": package.get("source", {}).get("url"),
                "declared_license": package["license"],
                "notices": notices,
            }
        )
    for source in policy["sources"]:
        name = source["name"]
        package = expected.get(name)
        if (
            package is None
            or package.get("source", {}).get("reference") != source["reference"]
            or package.get("license") != source["declared_license"]
        ):
            raise ValueError(f"Psalm source policy does not match the lock: {name}")
        archive = sources / source["file"]
        data = read(archive)
        if digest(data) != source["sha256"]:
            raise ValueError(f"Psalm source checksum mismatch: {name}")
        with tarfile.open(archive, "r:gz") as tar:
            licenses = [
                item
                for item in tar.getmembers()
                if item.isfile()
                and len(Path(item.name).parts) == 2
                and Path(item.name).name == "LICENSE"
            ]
            if len(licenses) != 1:
                raise ValueError(f"Psalm source archive has no unique LICENSE: {name}")
            notice = tar.extractfile(licenses[0]).read()
        if notice != read(vendor / name / "LICENSE"):
            raise ValueError(f"Psalm source and installed license differ: {name}")
        if name == "mortenson/psalm-plugin-drupal" and b"Commons Clause" not in notice:
            raise ValueError("Drupal Psalm license condition changed")
        copies[f"sources/{source['file']}"] = data
    for notice in policy["additional_notices"]:
        data = read(specification / notice["file"])
        if digest(data) != notice["sha256"]:
            raise ValueError("Psalm supplemental notice checksum mismatch")
        copies[notice["file"]] = data
    report = {
        "version": 1,
        "composer_lock_sha256": digest(lock_data),
        "runtime": policy["runtime"],
        "publication": policy["publication"],
        "sources": policy["sources"],
        "additional_notices": policy["additional_notices"],
        "packages": inventory,
    }
    if output.exists() and any(output.iterdir()):
        raise ValueError("Psalm notices output must be empty")
    output.mkdir(parents=True, exist_ok=True)
    for name, data in copies.items():
        path = output / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    shutil.copyfile(specification / "NOTICE.md", output / "NOTICE.md")
    (output / "inventory.json").write_text(json.dumps(report, indent=2) + "\n")
    return report


def main():
    parser = argparse.ArgumentParser(
        description="Verify installed Psalm dependencies and preserve their license evidence"
    )
    parser.add_argument("--bundle", type=Path, default=ROOT)
    parser.add_argument("--sources", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = prepare(args.bundle, args.sources, args.output)
    print(json.dumps({"packages": len(result["packages"]), "publication": result["publication"]}))


if __name__ == "__main__":
    main()
