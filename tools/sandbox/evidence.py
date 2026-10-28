import json
import re
from contextlib import contextmanager


def component(value):
    value = str(value)
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]*", value):
        raise ValueError("Invalid sandbox evidence component")
    return value


class Evidence:
    def __init__(self, sandbox):
        self.sandbox = sandbox
        self.root = sandbox.root / "evidence"
        self.identity = {
            "drupal": str(sandbox.summary.get("drupal_major", "native")),
            "environment": sandbox.summary["environment"],
        }
        self.cases, self.active = [], []

    @contextmanager
    def case(self, name, installation="shared"):
        directory = self.root.joinpath(
            *(
                component(value)
                for value in (
                    self.identity["drupal"],
                    self.identity["environment"],
                    installation,
                    name,
                )
            )
        )
        directory.mkdir(parents=True, exist_ok=False)
        record = {
            **self.identity,
            "installation": installation,
            "scenario": name,
            "status": "running",
            "commands": [],
        }
        self.cases.append(record)
        self.active.append((directory, record))
        try:
            yield record
        except BaseException as error:
            record.update(status="failed", error_type=type(error).__name__)
            self.sandbox.summary["passed"] = False
            raise
        else:
            record["status"] = "passed"
        finally:
            self.active.pop()
            record["result"] = str((directory / "result.json").relative_to(self.sandbox.root))
            (directory / "result.json").write_text(json.dumps(record, indent=2))

    def log_directory(self):
        if not self.active:
            return self.sandbox.logs
        path = self.active[-1][0] / "logs"
        path.mkdir(exist_ok=True)
        return path

    def command(self, label, code, timed_out=False, expected=(0,)):
        for _, record in self.active:
            record["commands"].append(
                {
                    "id": label,
                    "exit_code": code,
                    "timed_out": timed_out,
                    "expected_exit_codes": list(expected),
                    "logs": str(self.log_directory().relative_to(self.sandbox.root)),
                }
            )

    def finalize(self):
        if not self.cases or any(record["status"] != "passed" for record in self.cases):
            self.sandbox.summary["passed"] = False
        self.root.mkdir(exist_ok=True)
        (self.root / "index.json").write_text(
            json.dumps(
                {
                    "version": 1,
                    **self.identity,
                    "passed": self.sandbox.summary["passed"],
                    "cases": self.cases,
                },
                indent=2,
            )
        )
        self.sandbox.summary["evidence"] = "evidence/index.json"
