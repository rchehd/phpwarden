# Project tests and mutation testing

## Run project tests

`phpwarden test` runs the project's installed PHPUnit using the configured PHP worker.
Install the project's development dependencies first; PHPWarden does not install or
replace PHPUnit. Drupal projects typically need the matching `drupal/core-dev`.

```sh
phpwarden test
phpwarden test web/modules/custom/my_module/tests/src/Unit
phpwarden test --filter=testBoundary --output artifacts/phpunit
phpwarden test --testsuite=unit --format=json
```

By default, the Drupal profile finds `tests/src/Unit` under custom source roots.
Test selection is independent of static-check Git scope and exclusions. Explicit
paths replace discovered paths; `--testsuite` selects a native configured suite
instead. PHPUnit configuration discovery checks the Composer root's `phpunit.xml`,
then `phpunit.xml.dist`, then Drupal core's `phpunit.xml.dist`. `--phpunit-config`
overrides discovery. PHPUnit configuration, bootstrap and extensions remain native.

Configure defaults in `.phpwarden.yaml`:

```yaml
tests:
  phpunit: auto
  config: auto
  paths: [web/modules/custom/my_module/tests/src/Unit]
  timeout_seconds: 300
```

Paths are relative to the project root, including in monorepos. `tests.suite`
replaces `tests.paths`; set only one. `auto` finds PHPUnit in the Composer root's
configured vendor directory. `init` includes commented examples of these settings.

Tests execute in a private writable copy; the original project, Git metadata and
application containers are not mounted. Copies omit `.git`, `.ddev`, agent settings
and `node_modules`; symlinks outside the project and special files are rejected.
The worker has no network access. This first runner targets unit tests; database,
browser and application-service integration is a later step. A test requiring PHP
extensions absent from the selected worker reports an execution/test error.

`--output` must name a new directory. Without it, artifacts live under the user
cache in `phpwarden/tests/run-*`. Each run saves `report.json`, combined `junit.xml`,
per-path JUnit files, and PHPUnit stdout/stderr. JSON retains every testcase;
text shows the first ten failed tests and points to the complete artifacts.
JUnit can record several errors for one testcase; the report retains every error.
Empty selections, all-skipped suites, malformed/missing JUnit, stale project
inputs and bootstrap/runtime failures cannot produce a successful result.
Exit codes are 0 for passing tests, 1 for test failures/errors or coverage thresholds, 2 for invalid
configuration/selection, 3 for incomplete execution, and 130 for interruption.

Use `phpwarden test --coverage` for line coverage and method CRAP with the
worker's pinned PCOV driver (disabled for ordinary tests). PHPUnit 9–12 is supported.
All selected paths run together, so shared source coverage is combined by PHPUnit.
The isolated configuration keeps the native bootstrap and extensions. With the
Drupal core fallback, coverage includes custom source roots and excludes `tests`
directories; project-native source filters are otherwise preserved. Uncovered
source files are included. `tests.coverage.include` replaces the native source
filter; `exclude` adds project-relative files/directories. These use literal paths,
not static-check globs, and remain independent of test selection.

```yaml
tests:
  coverage:
    enabled: false
    include: [web/modules/custom/my_module/src]
    exclude: [web/modules/custom/my_module/src/Generated]
    min_line_coverage: 80
    max_crap: 30
```

Coverage and CRAP are advisory unless thresholds are configured. A threshold failure
returns 1 with `complete: true`; missing driver/reports or zero executable lines
return 3. Thresholds apply only when coverage is enabled. Lower CRAP is better:
it combines method complexity with how much code the selected tests cover. It is
not a mutation score. Reports use upstream php-code-coverage values, with method
positions from coverage XML and complexity from Crap4J. Procedural functions count
toward line coverage; the CRAP summary and gate currently cover class/trait methods.
A CRAP gate with no measurable methods is incomplete. PCOV provides line coverage,
not branch/path coverage.

Artifacts include `coverage/html/index.html`, native `coverage/xml/`, `clover.xml`
and `crap4j.xml` under `coverage/`, alongside JUnit and `report.json`. Text shows the
ten highest method CRAP scores; JSON retains every method and full totals.
Infection runs through the separate opt-in `mutation` command below.

## Mutation testing

`phpwarden mutation` uses the worker's pinned Infection 0.35.8 with the project's
installed PHPUnit. It supports PHP 8.3/8.4, initially targeting custom unit tests;
PHP 8.1/8.2 are rejected without substituting another PHP runtime. It runs only
when explicitly requested, outside static `check`, pre-commit and agent completion
hooks. Matching worker images are required (development toolset v30).

```sh
phpwarden mutation
phpwarden mutation web/modules/custom/my_module/src
phpwarden mutation --commits 2
phpwarden mutation --base origin/main --whole-files
phpwarden mutation --infection-config infection.json5 --format=json
```

Source scope defaults to **full**, using the resolved `scope.include` and
`scope.exclude`. Explicit paths are relative to the caller. `--mode diff`,
`--base`, `--commits` and `--whole-files` use the same Git selection as static
checks. This first integration mutates `.php` source files and excludes profile
test directories, initially targeting PHP class methods. Procedural free functions
are not assessed by the pinned Infection traversal; a zero-mutation result remains
incomplete. `tests.*`, `--testsuite` and `--filter` select tests independently.

The command runs initial tests and fresh coverage, then starts Infection only if
both pass. Coverage from another run is never reused. Everything runs in the same
private project copy, without network or original project mounts. Diff runs build
a private Git base from the selected source revisions; original Git metadata and
host Git configuration are not passed to Infection.

```yaml
mutation:
  config: auto
  timeout_seconds: 1800
  threads: 2
  mutant_timeout_seconds: 10
  mutators: ['@default']
  min_msi: 80
  min_covered_msi: 90
```

Defaults select automatic config discovery and a 1800-second overall timeout;
the mutator override list remains empty.
Without overrides, native threads/timeout/mutators are preserved; absent native
settings use one thread, ten seconds per mutant and `@default`. Explicit
`mutation.mutators` replaces native mutator selection; per-mutator ignores and
advanced options belong in the native file. MSI thresholds are advisory unless
configured; `.phpwarden.yaml` thresholds override native `minMsi`/`minCoveredMsi`.

`auto` discovers `infection.json5`, `infection.json`, then their `.dist` variants
at `composer_root`. Explicit native config paths start at the project root; native
source paths remain relative to that config's directory. Source excludes, mutator
profiles/ignores and bootstrap are retained. PHPWarden supplies PHPUnit selection,
artifact paths and a private temporary directory. Other test frameworks, raw
framework arguments, initial PHP flags and Infection static-analysis integrations
are currently rejected with a setup diagnostic; use `tests.*` for test selection.

Artifacts include `infection.json`, `infection.html`, fresh coverage/JUnit, native
stdout/stderr and the combined `report.json`. JSON preserves every mutant with its
source, changed code, diff and test output; text shows totals, MSI and up to ten
escaped mutants. MSI is a mutation score, distinct from line coverage and CRAP.

Exit 0 means a complete assessment with no failed threshold, even if advisory
escaped or uncovered mutants remain. Exit 1 means initial tests or an MSI threshold
failed. Missing/invalid reports, no assessed mutations, skipped mutants, execution
errors and timeouts return incomplete exit 3. A covered-MSI gate with no covered
mutations is incomplete. Source changes during execution invalidate the result.

The **Mutation tests** Actions workflow runs separate Drupal 10/11 × PHP
8.3/8.4 scenarios after Checks on relevant PRs and on manual dispatch. It builds matching workers and locked unit fixtures
without starting an application or database, and uploads only reports/logs.
