import argparse
import html
import json
from pathlib import Path


def cell(value):
    return html.escape(str(value)).replace("|", "&#124;").replace("\n", " ").replace("\r", " ")


def render(directories):
    rows, passed = [], True
    for directory in directories:
        try:
            summary = json.loads((directory / "summary.json").read_text())
            index = json.loads((directory / "evidence/index.json").read_text())
            cases = index["cases"]
            success = summary["passed"] is True and index["passed"] is True and bool(cases)
            success = success and all(case["status"] == "passed" for case in cases)
            passed = passed and success
            for case in cases:
                rows.append(
                    [
                        case["drupal"],
                        case["environment"],
                        case["installation"],
                        case["scenario"],
                        case["status"],
                        len(case["commands"]),
                    ]
                )
            if not cases:
                rows.append([index["drupal"], index["environment"], "shared", "run", "failed", 0])
        except (OSError, ValueError, KeyError, TypeError):
            passed = False
            rows.append(["unknown", directory.name, "shared", "evidence", "missing or invalid", 0])
    lines = [
        "### Sandbox results",
        "",
        "Overall: **" + ("PASS" if passed else "FAIL") + "**",
        "",
        "| Drupal | Environment | Installation | Scenario | Result | Commands |",
        "| --- | --- | --- | --- | --- | ---: |",
    ]
    lines += ["| " + " | ".join(cell(value) for value in row) + " |" for row in rows]
    lines += [
        "",
        "Download the matching sandbox artifact for result.json and numbered command logs.",
        "",
    ]
    return "\n".join(lines), passed


def main():
    parser = argparse.ArgumentParser(description="Summarize sandbox evidence for CI")
    parser.add_argument("directories", type=Path, nargs="+")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    text, passed = render(args.directories)
    if args.output:
        with args.output.open("a") as output:
            output.write(text)
    else:
        print(text)
    raise SystemExit(0 if passed else 1)


if __name__ == "__main__":
    main()
