import json
import os
import shutil
import stat
import subprocess
from pathlib import Path
from types import SimpleNamespace

APPLICATION = (
    "project",
    "unit-project",
    "data",
    "packages",
    "cache",
    "npm-cache",
    "dist",
    "bin",
    "php",
    "claude",
    "compose-worker",
    "compose-cleanup",
    "ddev-cache",
    "ddev-global",
    "certificates",
)
DOCKERLESS = ("project", "cache", "tmp", "helpers", "bin", "dist", "claude")


def active_sources():
    ids = subprocess.run(
        ["docker", "ps", "-q"], capture_output=True, text=True, timeout=15, check=True
    ).stdout.split()
    if not ids:
        return []
    result = subprocess.run(
        ["docker", "inspect", "--format", "{{json .Mounts}}", *ids],
        capture_output=True,
        text=True,
        timeout=15,
        check=True,
    )
    return [
        Path(mount["Source"])
        for line in result.stdout.splitlines()
        for mount in json.loads(line)
        if mount.get("Source")
    ]


def remove(root, names, *, keep=False, blocked=False, mounts=()):
    root = Path(root)
    names = tuple(names)
    if root.is_symlink() or any(
        not name or Path(name).name != name or name in (".", "..") for name in names
    ):
        raise ValueError("Workspace cleanup requires a real root and direct child names")
    result = {"removed": [], "retained": [], "errors": []}
    if keep or blocked:
        result.update(
            status="retained", reason="resource cleanup failed" if blocked else "requested"
        )
        result["retained"] = [
            name for name in names if (root / name).exists() or (root / name).is_symlink()
        ]
        return result
    for name in names:
        target = root / name
        if any(source.is_relative_to(target) or target.is_relative_to(source) for source in mounts):
            result["errors"].append({"path": name, "error_type": "ActiveMount"})
            continue
        try:
            if target.is_symlink() or target.is_file():
                target.unlink()
            elif target.is_dir():
                for directory, _, _ in os.walk(target, topdown=True, followlinks=False):
                    current = Path(directory)
                    info = current.lstat()
                    if info.st_uid == os.getuid() and not current.is_symlink():
                        os.chmod(current, stat.S_IMODE(info.st_mode) | stat.S_IRWXU)
                shutil.rmtree(target)
            else:
                continue
            result["removed"].append(name)
        except OSError as error:
            result["errors"].append({"path": name, "error_type": type(error).__name__})
    result["status"] = "failed" if result["errors"] else "removed"
    return result


def finish(sandbox, names, *, blocked=False, check_mounts=False):
    try:
        mounts = (
            active_sources() if check_mounts and not blocked and not sandbox.keep_workspace else ()
        )
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        result = remove(sandbox.root, names, blocked=True)
        result.update(status="failed", reason="resource inspection failed")
        result["errors"] = [{"error_type": type(error).__name__}]
        sandbox.summary.update(passed=False, workspace_cleanup=result)
        raise RuntimeError("Cannot verify workspace resources; preserving it") from error
    result = remove(
        sandbox.root, names, keep=sandbox.keep_workspace, blocked=blocked, mounts=mounts
    )
    sandbox.summary["workspace_cleanup"] = result
    if result["errors"]:
        sandbox.summary["passed"] = False
        raise RuntimeError("Workspace cleanup failed; see summary.json")


def startup_failure(root, names, keep):
    summary = {"passed": False}
    try:
        finish(SimpleNamespace(root=root, summary=summary, keep_workspace=keep), names)
    finally:
        (root / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
