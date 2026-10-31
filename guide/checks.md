# Running checks and reading results

## Run checks

```sh
phpwarden check                                # changed lines against scope.base
phpwarden check --base origin/develop          # another base branch
phpwarden check --commits 2                    # the last 2 commits plus uncommitted changes
phpwarden check --whole-files                  # all lines of changed files
phpwarden check --mode full                    # all custom code
phpwarden check web/modules/custom/my_module   # specific paths
phpwarden check --only phpstan,phpcs           # selected checkers
phpwarden check --format compact               # one path:line:col line per finding
phpwarden check --format json                  # JSON report
phpwarden check --all                          # every finding in each text rule group
```

Changed code includes commits since the merge base, staged and unstaged edits, and new files not ignored by Git. Without `--base` or `scope.base`, PHPWarden uses `origin/HEAD`. It never fetches, so update the base branch yourself.

To see which files and lines a check would cover:

```sh
phpwarden scope
```

Checks mount the project read-only. To apply PHPCS and Twig CS Fixer corrections explicitly:

```sh
phpwarden check --fix
phpwarden check --fix --only phpcs web/modules/custom/my_module
phpwarden fix --only twig-cs-fixer web/themes/custom/my_theme
```

`fix` is an alias for `check --fix`. Without `--only`, fixes use the enabled PHPCS and Twig CS Fixer settings, then run the configured checks again. An explicit `--only` accepts these two formatters and limits the final checks to them; requesting another checker with `--fix` is an input error.

Fixes may change any line in the selected files, including lines outside diff hunks. The final check covers all lines of the original selection. Formatters prepare edits with the project still mounted read-only; the launcher validates input hashes and applies replacements on the host, preserving permission bits. Concurrent changes detected before writing abort the operation. Each file replacement is atomic; a later write failure can leave earlier replacements applied. JSON reports list those files in `fixed_files`, and text reports show the same list. Other findings can remain after formatting and determine the final exit code.

## Results

Interactive text runs show a live stage and elapsed time on stderr while preparing the project, running Docker checks and fetching Composer advisories. The status line is cleared before the final report. Redirected output, CI, JSON, compact output and `TERM=dumb` do not animate. The Docker worker currently returns its checks together, so the launcher shows the Docker analysis stage rather than individual checker progress or a percentage.

The default text output starts with the result and analysis completeness, then organizes findings and execution problems into categories. Each analyzer belongs to one category; the category is used for both details and the final checklist. Tool names and native rule identifiers are omitted from headings and checklist rows. Diagnostic messages retain the configuration paths and details needed to resolve a problem.

| Category | Checks |
|---|---|
| Security | Secrets, security rules, dependency advisories |
| Code standards | PHP, JavaScript, stylesheet and Twig standards; whitespace |
| Correctness | Syntax, types, API usage and debug leftovers |
| Dependencies | Package validity, undeclared and unused dependencies |
| Maintainability | Duplication, complexity and dead code |
| Drupal quality | Module configuration and caching |
| Architecture | Dependency boundaries |
| Upgrades | Modernization and deprecation replacements |

Project and execution-wide problems appear under Environment. Within each category, errors precede warnings and informational findings. Repeated explanations appear once; differing messages keep their details next to the relevant positions:

```text
PHPWarden · changes against origin/main · 3 files · Drupal 11.3.16 · PHP 8.4.26
Result: FAIL · 2 errors · exit 1 (blocking findings)
Analysis: complete · 1 category

Code standards

  ERROR Duplicate spaces — 2 findings · 1 file

    packages/ap-related-content/ap_related_content.module
      71:24 · 72:23

Checks
  ✗ Code standards  failed       2 errors
```

Within a category, each rule group shows the first 10 findings by default, with an explicit count of the remaining findings and affected files. `--all` shows every finding; this affects display only, never analysis, totals, caching or exit codes. Category status preserves required failures, optional failures and skipped coverage: a successful subcheck never hides a failing one. A missing native configuration makes mixed coverage partial; disabled or inapplicable checks are listed as skips. Messages and position lists wrap at 100 characters where words allow it; paths and long identifiers remain intact.

`--format compact` retains technical checker/rule identifiers and prints every finding on one `path:line:col` line, which editors and terminals can open directly, followed by its summary. If the check stops before any checker runs, the output says so instead of reporting zero findings. Colors are used only in a terminal; `--color always|never` overrides that, and `NO_COLOR` disables it.

`--format json` prints one report that follows [schemas/report-v1.schema.json](../schemas/report-v1.schema.json). Its content and the exit codes do not depend on the text format. Each checker that ran live has an optional `duration_ms`, and the text output lists the three slowest.

| Exit code | Meaning |
|---|---|
| `0` | No blocking findings |
| `1` | Blocking findings |
| `2` | Invalid configuration, arguments, or project metadata |
| `3` | A required checker could not run, or the project changed during the check |
| `130` | Interrupted |

`PARTIAL` in text output (`complete: false` in JSON) means an optional checker could not run. `Discovery warning` lines report problems in project metadata that did not stop the check, such as duplicate keys in `composer.json` or a `vendor` that does not match `composer.lock`.

### Analyzer compatibility notes

PHPStan trait diagnostics may name a file with a class-context suffix; PHPWarden validates the physical selected file and merges those contextual messages. Rector runs from `composer_root` so its installed-package resolver sees the correct Composer metadata in a monorepo. JSCPD 4.3.0 matching is constrained to consecutive reference token windows to avoid joining unrelated repeated YAML blocks into reversed ranges.

Gitleaks scans bounded private copies of the selected files, retaining their relative paths for native baselines and allowlists. Declared `extra_inputs` are exposed as config-only symlinks, with native symlink scanning explicitly disabled. No unselected source files are added to the scan; private input copies and reports are removed after completion.

ESLint and Stylelint project configs use installed project dependencies. When a Node config import is missing, the checker reports an unavailable dependency with the relevant `npm ci --prefix` command; it does not silently substitute a different plugin or treat the result as clean.
