import argparse
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
CASES = {"direct-html": ["phpwarden.php.request-xss"], "cross-file-html": [], "escaped-html": []}


def main():
    parser = argparse.ArgumentParser(
        description="Compare Psalm fixtures with bundled Semgrep CE rules"
    )
    parser.add_argument("--reports", type=Path, required=True, help="Fresh spike output directory")
    parser.add_argument(
        "--semgrep-core", type=Path, required=True, help="Pinned prepared Semgrep engine"
    )
    parser.add_argument("--library-dir", type=Path, help="Native bundle libraries, if needed")
    args = parser.parse_args()
    binary = args.semgrep_core.resolve(strict=True)
    environment = os.environ.copy()
    if args.library_dir is not None:
        environment["LD_LIBRARY_PATH"] = str(args.library_dir.resolve(strict=True))
    version = subprocess.run(
        [str(binary), "-version"],
        capture_output=True,
        text=True,
        timeout=10,
        check=True,
        env=environment,
    ).stdout.strip()
    if not version.startswith("semgrep-core version: 1.182.0 ("):
        parser.error(f"Unexpected engine: {version}")
    summary = json.loads((args.reports / "summary.json").read_text())
    if len(summary["cases"]) != 6 or not all(case["passed"] for case in summary["cases"]):
        parser.error("Psalm spike did not pass all six controls")
    rows = []
    for case, expected in CASES.items():
        matches = []
        with tempfile.TemporaryDirectory(prefix="phpwarden-taint-compare-") as temporary:
            target = Path(temporary) / "target.php"
            rules = Path(temporary) / "rules.json"
            rules.write_text(
                json.dumps(
                    yaml.safe_load((ROOT / "internal/checker/semgrep/rules.yaml").read_text())
                )
            )
            for source in sorted((args.reports / case / "src").iterdir()):
                shutil.copyfile(source, target)
                result = subprocess.run(
                    [
                        str(binary),
                        "-rules",
                        str(rules),
                        "-lang",
                        "php",
                        "-j",
                        "1",
                        "-json_time",
                        "-json_nodots",
                        "-strict",
                        "-no_filter_irrelevant_rules",
                        "-timeout",
                        "10",
                        "-timeout_threshold",
                        "1",
                        "-max_memory",
                        "512",
                        "-max_match_per_file",
                        "10000",
                        str(target),
                    ],
                    capture_output=True,
                    text=True,
                    timeout=30,
                    check=True,
                    env=environment,
                )
                log = args.reports / case / ("semgrep-" + source.name)
                log.with_suffix(log.suffix + ".json").write_text(result.stdout)
                log.with_suffix(log.suffix + ".stderr").write_text(result.stderr)
                report = json.loads(result.stdout)
                if (
                    report.get("errors")
                    or report.get("skipped_targets")
                    or report.get("skipped_rules")
                    or result.stderr
                ):
                    raise RuntimeError(f"Semgrep scan incomplete: {case}/{source.name}")
                matches.extend(match["check_id"] for match in report["results"])
        psalm = json.loads((args.reports / case / "stdout.json").read_text())
        rows.append(
            {
                "case": case,
                "psalm": sorted({issue["type"] for issue in psalm}),
                "semgrep": sorted(matches),
                "passed": sorted(matches) == expected,
            }
        )
    result = {"engine": version, "cases": rows}
    (args.reports / "comparison.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
    return 0 if all(row["passed"] for row in rows) else 1


if __name__ == "__main__":
    raise SystemExit(main())
