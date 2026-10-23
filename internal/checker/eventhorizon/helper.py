import contextlib
import importlib
import io
import json
import logging
import sys
from pathlib import Path


class AnalysisError(RuntimeError):
    pass


class StrictLogs(logging.Handler):
    def emit(self, record):
        if record.levelno >= logging.WARNING:
            raise AnalysisError("EventHorizon reported an incomplete analysis")


def analyze(request, bundle, root):
    metadata = json.loads((bundle / "source.json").read_text())
    sys.path.insert(0, str(bundle))
    eventhorizon = importlib.import_module("eventhorizon")
    yaml = importlib.import_module("yaml")
    caching = importlib.import_module("eventhorizon.scanner.caching_analyzer")
    config_analysis = importlib.import_module("eventhorizon.scanner.config_analyzer")
    config_validation = importlib.import_module("eventhorizon.scanner.config_validator")
    if (
        metadata["version"] != request["tool_version"]
        or eventhorizon.__version__ != metadata["version"]
        or metadata["revision"] != request["revision"]
        or metadata["pyyaml"] != yaml.__version__
        or yaml.__version__ != request["pyyaml"]
    ):
        raise AnalysisError("EventHorizon bundle version mismatch")
    logger = logging.getLogger("EventHorizon")
    logger.handlers = [StrictLogs()]
    logger.setLevel(logging.WARNING)
    logger.propagate = False

    class StrictLoader(yaml.SafeLoader):
        def construct_mapping(self, node, deep=False):
            self.flatten_mapping(node)
            keys = set()
            for key_node, _ in node.value:
                key = self.construct_object(key_node, deep=deep)
                if key in keys:
                    raise AnalysisError("Duplicate config key")
                keys.add(key)
            return super().construct_mapping(node, deep)

    findings = []
    for name in request["files"]:
        file = root / name
        content = file.read_text(encoding="utf-8", errors="strict")
        if "\x00" in content:
            raise AnalysisError("Invalid source text")
        for issue in caching.SmartCachingAnalyzer().analyze_file(file):
            if issue.get("type") == "file_error" or Path(issue["file"]) != file:
                raise AnalysisError("Invalid caching result")
            findings.append(
                {
                    "file": name,
                    "line": issue["line"],
                    "rule": "caching." + issue["rule"],
                    "severity": issue["severity"],
                    "message": issue["message"],
                }
            )
    for directory in request["config_dirs"]:
        path = root / directory
        for file in sorted(path.glob("*.yml")):
            data = yaml.load(file.read_text(encoding="utf-8", errors="strict"), Loader=StrictLoader)
            if not isinstance(data, dict):
                raise AnalysisError("Exported config must be a mapping")
        analyzer = config_analysis.DrupalConfigAnalyzer(str(path))
        validator = config_validation.DrupalConfigValidator(analyzer.analyze_all_configs())
        for issue in validator.validate_all():
            message = issue.message
            if issue.recommendation:
                message += " " + issue.recommendation
            findings.append(
                {
                    "file": directory,
                    "line": 0,
                    "rule": "config." + issue.issue_type,
                    "severity": issue.severity,
                    "message": message,
                }
            )
    return {
        "version": 1,
        "tool_version": metadata["version"],
        "revision": metadata["revision"],
        "pyyaml": yaml.__version__,
        "files": request["files"],
        "config_dirs": request["config_dirs"],
        "findings": findings,
    }


def main():
    try:
        request = json.load(sys.stdin)
        captured_out, captured_err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(captured_out), contextlib.redirect_stderr(captured_err):
            result = analyze(request, Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve())
        if captured_out.getvalue() or captured_err.getvalue():
            raise AnalysisError("Unexpected analyzer output")
        print(json.dumps(result, ensure_ascii=True))
    except Exception:
        print(
            "EventHorizon could not complete its analysis; no partial findings accepted",
            file=sys.stderr,
        )
        return 70
    return 0


if __name__ == "__main__":
    sys.exit(main())
