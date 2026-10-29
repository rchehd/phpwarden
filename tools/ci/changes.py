import argparse
import json
import os
import re
import subprocess
from pathlib import Path

GROUPS = ("go", "python", "dockerless", "sandbox", "mutation")
MUTATION_CORE = (
    "internal/cli/",
    "internal/config/",
    "internal/discovery/",
    "internal/launcher/",
    "internal/profile/",
    "internal/progress/",
    "internal/projectpath/",
    "internal/projecttest/",
    "internal/runner/",
    "internal/scope/",
    "internal/toolset/",
)
DOCKERLESS_CORE = (
    "internal/cli/",
    "internal/distribution/",
    "internal/selfupdate/",
    "internal/legal/",
    "internal/performance/",
    "internal/perfmcp/",
    "internal/perfruntime/",
    "internal/agentskills/",
    "internal/agenthook/",
    "internal/githook/",
)


def classify(paths, force=False):
    selected = {group: force for group in GROUPS}

    def select(*groups):
        for group in groups:
            selected[group] = True

    for name in paths:
        if name in ("requirements-dev.txt", "pyproject.toml", "ruff.toml", ".ruff.toml"):
            select("python")
            continue
        if name.startswith(("docs/", ".ai/", ".agents/")) or (
            name.endswith(".md")
            and (
                "/" not in name
                or name.startswith("tools/")
                and name.count("/") == 2
                and name.rsplit("/", 1)[-1] == "README.md"
            )
        ):
            continue
        if name in (
            "go.mod",
            "go.sum",
            "docker-bake.hcl",
            "docker-bake.json",
            ".dockerignore",
            ".gitignore",
            ".gitattributes",
            ".github/workflows/ci.yml",
        ) or name.startswith(("docker/", "schemas/", "tools/ci/", ".github/actions/changes/")):
            select(*GROUPS)
        elif name.startswith(".github/workflows/"):
            select("go", "python")
            if name == ".github/workflows/sandbox.yml":
                select("sandbox", "dockerless")
            if name == ".github/workflows/mutation.yml":
                select("mutation")
        elif name.endswith(".go") or name.startswith("cmd/"):
            select("go")
            if not name.endswith("_test.go"):
                select("sandbox")
                if "/" not in name or name.startswith(("cmd/", *MUTATION_CORE)):
                    select("mutation")
                if "/" not in name or name.startswith(("cmd/", *DOCKERLESS_CORE)):
                    select("dockerless")
        elif name.startswith("internal/"):
            select("go", "sandbox")
            if name.endswith(".py"):
                select("python")
            if name.startswith(MUTATION_CORE):
                select("mutation")
            if name.startswith(DOCKERLESS_CORE):
                select("dockerless")
        elif name.startswith("tests/"):
            select("python")
            if name.startswith(("tests/fixtures/", "tests/support/", "tests/acceptance/")):
                select("go")
            if name in ("tests/acceptance/launcher_cli.py", "tests/acceptance/compose_cli.py"):
                select("sandbox")
        elif name.startswith("tools/sandbox/"):
            if name.endswith(".py"):
                select("python")
            relative = name.removeprefix("tools/sandbox/")
            if relative in ("mutation.py", "mutation_tests.py"):
                select("mutation")
            elif relative == "dockerless.py":
                select("dockerless")
            else:
                select("sandbox")
                if relative in (
                    "assertions.py",
                    "project_tests.py",
                    "Dockerfile",
                ) or relative.startswith(
                    ("fixture/", "unit-fixture/", "drupal10-tests/", "drupal11-tests/")
                ):
                    select("mutation")
                if relative in (
                    "assertions.py",
                    "commands.py",
                    "evidence.py",
                    "mcp.py",
                    "report.py",
                ):
                    select("dockerless")
        elif name.startswith("tools/spx/"):
            select("go", "sandbox", "dockerless")
            if name.endswith(".py"):
                select("python")
        elif name in ("tools/distribution.py", "tools/licenses.py"):
            select("go", "python", "sandbox", "dockerless")
        elif name.startswith("tools/"):
            select("go", "sandbox", "mutation")
            if name.endswith(".py"):
                select("python")
        elif name == "install.sh" or name.startswith("examples/"):
            select("go", "sandbox", "dockerless")
    return selected


def changed_paths(event, event_name, root):
    if event_name == "pull_request":
        base = event.get("pull_request", {}).get("base", {}).get("sha", "")
        head = event.get("pull_request", {}).get("head", {}).get("sha", "")
        separator = "..."
    elif event_name == "push":
        if event.get("deleted"):
            return [], "deleted branch"
        base, head, separator = event.get("before", ""), event.get("after", ""), ".."
    else:
        return None, "manual or reusable invocation"
    if not all(re.fullmatch(r"[0-9a-f]{40,64}", value) for value in (base, head)):
        raise ValueError("Missing or invalid GitHub comparison commits")
    if set(base) == {"0"}:
        return None, "new branch has no previous commit"
    result = subprocess.run(
        ["git", "diff", "--name-only", "--no-renames", "-z", base + separator + head, "--"],
        cwd=root,
        capture_output=True,
        check=False,
    )
    if result.returncode:
        return None, "comparison history unavailable; run all checks"
    return [os.fsdecode(name) for name in result.stdout.split(b"\0") if name], "changed files"


def main():
    parser = argparse.ArgumentParser(description="Select CI jobs from changed repository paths")
    parser.add_argument("--paths", nargs="*")
    parser.add_argument("--force-all", action="store_true")
    args = parser.parse_args()
    force = args.force_all or os.environ.get("CI_FORCE_ALL", "false").lower() == "true"
    if args.paths is not None:
        paths, reason = args.paths, "explicit paths"
    elif force:
        paths, reason = [], "full validation requested"
    else:
        event_path = os.environ.get("GITHUB_EVENT_PATH")
        if not event_path:
            parser.error("GITHUB_EVENT_PATH or --paths is required")
        paths, reason = changed_paths(
            json.loads(Path(event_path).read_text()), os.environ["GITHUB_EVENT_NAME"], Path.cwd()
        )
    selected = classify(paths or [], force=force or paths is None)
    output = os.environ.get("GITHUB_OUTPUT")
    if output:
        with Path(output).open("a") as stream:
            for group, enabled in selected.items():
                stream.write(f"{group}={str(enabled).lower()}\n")
    print(json.dumps({"reason": reason, "files": paths, "selected": selected}, indent=2))


if __name__ == "__main__":
    main()
