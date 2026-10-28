import hashlib
import json
import stat
from contextlib import contextmanager
from pathlib import Path


def container_snapshot(inspection, started=False):
    result = {
        key: inspection[key] for key in ("Id", "Image", "Config", "HostConfig", "RestartCount")
    }
    result["Mounts"] = sorted(
        inspection["Mounts"], key=lambda mount: json.dumps(mount, sort_keys=True)
    )
    result["running"] = inspection["State"]["Running"]
    if started:
        result["started"] = inspection["State"]["StartedAt"]
    return result


def snapshot(paths):
    result = {}
    for path in paths:
        if not path.exists() and not path.is_symlink():
            result[str(path)] = None
            continue
        info = path.lstat()
        value = {"uid": info.st_uid, "gid": info.st_gid, "mode": stat.S_IMODE(info.st_mode)}
        if path.is_symlink():
            value["link"] = str(path.readlink())
        else:
            value["sha256"] = hashlib.sha256(path.read_bytes()).hexdigest()
        result[str(path)] = value
    return result


@contextmanager
def preserved(paths):
    paths = list(paths)
    before = snapshot(paths)
    try:
        yield
    finally:
        assert before == snapshot(paths), "Preserved fixture files changed"


def tree_snapshot(directory):
    paths = [
        path
        for path in directory.rglob("*")
        if ".git" not in path.relative_to(directory).parts and (path.is_file() or path.is_symlink())
    ]
    return {
        str(Path(name).relative_to(directory)): value for name, value in snapshot(paths).items()
    }
