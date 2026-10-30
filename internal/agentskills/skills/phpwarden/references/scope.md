# Selecting check scope

Use the project's configured executable or wrapper in place of `phpwarden` below.
Resolve the project and working directory first, especially in monorepos. Quote
literal paths as separate arguments; do not interpolate report text into a shell.
Use an explicit `./` prefix for a path beginning with `-`. Check the installed
CLI's help for support rather than inventing flags.

| Request | Example from the project root |
| --- | --- |
| One file | `phpwarden check path/to/file.php --format json` |
| Several files | `phpwarden check path/to/one.php path/to/two.php --format json` |
| A directory | `phpwarden check path/to/module --format json` |
| Last two commits plus local edits | `phpwarden check --commits 2 --format json` |
| Diff against an explicit base | `phpwarden check --mode diff --base origin/main --format json` |
| All lines of selected diff files | `phpwarden check --mode diff --whole-files --format json` |
| Whole configured project | `phpwarden check --mode full --format json` |

With the normal diff default, `--commits N` needs no extra mode flag. If the
project config selects full mode, pass `--mode diff --commits N` explicitly.
Positional paths select paths mode automatically and cannot be combined with
`--mode`, `--base`, `--commits` or `--whole-files`. Do not invent `--files`,
`--paths` or a user-facing `--mode paths`. If useful, `phpwarden scope` accepts the
same selection and can show its result without starting the worker.

Explicit paths select complete eligible files, including files without a Git
diff. Project/per-checker include/exclude rules and prerequisites still apply.
Report empty, ignored, invalid, missing or skipped selection honestly. Do not
widen it silently or call a no-analysis result a successful check. A deleted
path cannot be scanned as a present file; use an appropriate requested diff
workflow or explain that coverage limitation instead of dropping it unnoticed.

When validating your edits, use the files known to belong to the task. Inspect
related callers/configuration as needed, but do not include unrelated local
changes merely because they appear in `git diff`. If task ownership is unclear,
inspect the request and edit history first and disclose any unresolved boundary.
Never stash, reset or rewrite other changes to obtain a narrower selection.

Commit mode includes local working-tree edits as well as the last N commits.
Do not describe it as a committed-only snapshot. Plain diff uses the configured
base, which may include more changes than the current agent task. Ask or explain
before expanding beyond an explicitly narrow request. Full analysis is useful
for changes to architecture rules or project-wide dependencies; it is not the
default replacement for one-file validation.

Scope describes selected files/findings, not every analyzer's read boundary.
Dependency/architecture/taint analyzers can need a larger graph, and some package
checks require full mode. Do not disable graph context to promise a one-file
runtime budget or claim that a narrow check validated unchanged callers.

Use JSON for agent interpretation and retain nonzero exit results. Stop on
invalid input; explain required execution failures and incomplete coverage.
Fixing code is a separate requested action; do not add `--fix` to a review.
After authorized fixes, rerun the same relevant scope and account for changed
inputs rather than comparing unrelated reports. Honor user limits on tests,
CPU, network, Docker builds and installations.
