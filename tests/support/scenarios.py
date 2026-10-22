import json
import os
import shutil
import subprocess
import tempfile
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path


@dataclass
class GitScenario:
    root: Path
    spec: dict
    env: dict
    base: str

    def git(self, *arguments):
        return subprocess.run(
            ["git", *arguments],
            cwd=self.root,
            env=self.env,
            capture_output=True,
            check=True,
            timeout=15,
        ).stdout

    def changed_files(self):
        tracked = self.git("diff", "--name-only", "--diff-filter=d", "-z", self.base)
        untracked = self.git("ls-files", "--others", "--exclude-standard", "-z")
        return sorted({os.fsdecode(path) for path in (tracked + untracked).split(b"\0") if path})


def load_spec(directory):
    spec = json.loads((directory / "scenario.json").read_text())
    if spec.get("version") != 1 or spec.get("id") != directory.name:
        raise ValueError(f"Invalid scenario identity: {directory}")
    state = spec.get("state")
    if state not in {"unstaged", "staged", "committed", "untracked"}:
        raise ValueError(f"Invalid scenario state: {state}")
    if state == "untracked":
        if not (directory / "untracked").is_dir() or "patch" in spec:
            raise ValueError(f"Invalid untracked scenario: {directory}")
    elif spec.get("patch") != "change.patch" or not (directory / "change.patch").is_file():
        raise ValueError(f"Missing scenario patch: {directory}")
    return spec


@contextmanager
def materialize(directory):
    directory = Path(directory).resolve()
    spec = load_spec(directory)
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    env.update(
        {
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_CONFIG_GLOBAL": os.devnull,
            "GIT_AUTHOR_NAME": "PHPWarden fixture",
            "GIT_AUTHOR_EMAIL": "fixture@example.invalid",
            "GIT_COMMITTER_NAME": "PHPWarden fixture",
            "GIT_COMMITTER_EMAIL": "fixture@example.invalid",
        }
    )
    with tempfile.TemporaryDirectory(prefix="phpwarden-test-") as temporary:
        root = Path(temporary)
        shutil.copytree(directory / "before", root, dirs_exist_ok=True)
        scenario = GitScenario(root, spec, env, "")
        scenario.git("init", "--quiet", "--initial-branch=main")
        scenario.git("config", "core.autocrlf", "false")
        scenario.git("add", "--all")
        scenario.git("commit", "--quiet", "-m", "Fixture baseline")
        scenario.base = scenario.git("rev-parse", "HEAD").decode().strip()
        scenario.git("switch", "--quiet", "-c", "scenario")
        if spec["state"] == "untracked":
            shutil.copytree(directory / "untracked", root, dirs_exist_ok=True)
        else:
            scenario.git("apply", "--check", str(directory / spec["patch"]))
            scenario.git("apply", str(directory / spec["patch"]))
            if spec["state"] in {"staged", "committed"}:
                scenario.git("add", "--all")
            if spec["state"] == "committed":
                scenario.git("commit", "--quiet", "-m", "Fixture change")
        yield scenario
