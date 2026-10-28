import subprocess
from contextlib import nullcontext


class Commands:
    def run(self, args, expected=(0,), cwd=None, input=None, timeout=180):
        self.sequence += 1
        label = f"{self.sequence:03}"
        evidence = getattr(self, "evidence", None)
        logs = evidence.log_directory() if evidence else self.logs
        try:
            result = subprocess.run(
                [str(arg) for arg in args],
                cwd=cwd or self.project,
                env=self.environment,
                input=input,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
        except subprocess.TimeoutExpired as error:
            for suffix, data in (("stdout", error.stdout), ("stderr", error.stderr)):
                if isinstance(data, bytes):
                    data = data.decode(errors="replace")
                (logs / (label + "." + suffix)).write_text(data or "")
            if evidence:
                evidence.command(label, None, timed_out=True, expected=expected)
            raise
        (logs / (label + ".stdout")).write_text(result.stdout)
        (logs / (label + ".stderr")).write_text(result.stderr)
        if evidence:
            evidence.command(label, result.returncode, expected=expected)
        if result.returncode not in expected:
            raise RuntimeError(
                f"Command {label} exited {result.returncode}: {args[:4]}\n"
                f"{result.stdout[-2500:]}\n{result.stderr[-2500:]}"
            )
        return result.stdout

    def cli(self, *args, expected=(0,), input=None, timeout=180):
        return self.run([self.binary, *args], expected=expected, input=input, timeout=timeout)

    def case(self, name, installation="shared"):
        evidence = getattr(self, "evidence", None)
        return evidence.case(name, installation) if evidence else nullcontext()

    def cleanup_steps(self, steps):
        errors = []
        for name, action in steps:
            try:
                with self.case("cleanup-" + name):
                    action()
            except Exception as error:
                errors.append({"step": name, "error_type": type(error).__name__})
        if errors:
            self.summary.update(passed=False, cleanup_errors=errors)
        return errors
