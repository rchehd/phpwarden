import argparse
import json
import os
import shutil
import subprocess
import uuid
from pathlib import Path

from tools.sandbox.assertions import tree_snapshot
from tools.sandbox.mutation_tests import mutation_tests
from tools.sandbox.project_tests import prepare_unit_project

ROOT = Path(__file__).resolve().parents[2]


class MutationSandbox:
    def __init__(self, args):
        self.root = args.output.resolve()
        self.root.mkdir(parents=True, exist_ok=False)
        self.project = self.root / "project"
        self.project.mkdir()
        self.major = args.drupal
        self.repository = ROOT
        self.image = args.composer_image
        self.binary = args.binary.resolve()
        self.name = "phpwarden-mutation-" + uuid.uuid4().hex[:12]
        self.logs = self.root / "logs"
        self.logs.mkdir()
        self.evidence = self
        self.sequence = 0
        self.summary = {"passed": False, "drupal": self.major, "php": args.runtime}
        fixture = ROOT / f"tools/sandbox/drupal{self.major}"
        for name in ("composer.json", "composer.lock"):
            shutil.copyfile(fixture / name, self.project / name)
        shutil.copytree(
            ROOT / "tools/sandbox/fixture", self.project / "web/modules/custom/phpwarden_probe"
        )
        (self.project / ".phpwarden.yaml").write_text(
            f'version: 1\nprofile: drupal\nphp: {{runtime: "{args.runtime}", '
            f'target: {{min: "{args.runtime}", max: "{args.runtime}"}}}}\n'
        )

    def log_directory(self):
        return self.logs

    def stage(self, text):
        print(f"Drupal {self.major}: {text}", flush=True)

    def run(self, arguments, cwd=None, expected=(0,), timeout=600):
        self.sequence += 1
        result = subprocess.run(
            [str(argument) for argument in arguments],
            cwd=cwd or self.project,
            env=os.environ,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        (self.logs / f"{self.sequence:03}.stdout").write_text(result.stdout)
        (self.logs / f"{self.sequence:03}.stderr").write_text(result.stderr)
        print(f"Command {self.sequence}: exit {result.returncode}", flush=True)
        if result.returncode not in expected:
            raise RuntimeError(
                f"Unexpected exit {result.returncode}: {result.stdout[-3000:]}\n"
                f"{result.stderr[-2000:]}"
            )
        return result.stdout

    def execute(self):
        before = tree_snapshot(self.project)
        try:
            prepare_unit_project(self)
            mutation_tests(self)
            assert before == tree_snapshot(self.project)
            self.summary["passed"] = True
        finally:
            (self.root / "summary.json").write_text(json.dumps(self.summary, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description="Opt-in isolated Drupal mutation acceptance")
    parser.add_argument("--drupal", choices=("10", "11"), required=True)
    parser.add_argument("--runtime", choices=("8.3", "8.4"), required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--composer-image", required=True)
    parser.add_argument("--output", type=Path, required=True)
    sandbox = MutationSandbox(parser.parse_args())
    sandbox.execute()
    print(json.dumps(sandbox.summary, indent=2), flush=True)


if __name__ == "__main__":
    main()
