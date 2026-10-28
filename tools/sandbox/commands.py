import subprocess


class Commands:
    def run(self, args, expected=(0,), cwd=None, input=None, timeout=180):
        self.sequence += 1
        result = subprocess.run(
            [str(arg) for arg in args],
            cwd=cwd or self.project,
            env=self.environment,
            input=input,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        label = f"{self.sequence:03}"
        (self.logs / (label + ".stdout")).write_text(result.stdout)
        (self.logs / (label + ".stderr")).write_text(result.stderr)
        if result.returncode not in expected:
            raise RuntimeError(
                f"Command {label} exited {result.returncode}: {args[:4]}\n"
                f"{result.stdout[-2500:]}\n{result.stderr[-2500:]}"
            )
        return result.stdout

    def cli(self, *args, expected=(0,), input=None, timeout=180):
        return self.run([self.binary, *args], expected=expected, input=input, timeout=timeout)
