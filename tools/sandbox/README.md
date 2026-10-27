# Installed Drupal sandboxes

This suite installs real Drupal 10.6.0 and 11.4.1 sites from committed Composer
locks, each with its own SQLite database and a controlled custom module. It
complements checker fixtures and package-manager installation tests; it tests
the application lifecycle that those smaller fixtures do not exercise.

Both fixtures use PHP 8.3 on Linux amd64. The version pins are compatibility
test cases, not production version recommendations. The local fixture-only
Composer advisory override allows reproducing historical versions; it does
not disable PHPWarden security checks. Dependency installation needs network
access. The installed application runs without network or exposed host ports.
Fixture manifests contain no project scripts. Only their explicitly allowed,
locked installer/scaffold plugins run; Drupal 11 also needs its Symfony
runtime plugin to generate `vendor/autoload_runtime.php` for the HTTP entrypoint.

## Run locally

Build the matching PHP 8.3 worker, then run either sandbox from the repo root:

```sh
docker buildx bake php83
python3 tools/sandbox/run.py --drupal 10 --output /tmp/phpwarden-sandbox-d10
python3 tools/sandbox/run.py --drupal 11 --output /tmp/phpwarden-sandbox-d11
```

The output directory must not exist. The script builds a development CLI
archive and installs it through the real `install.sh`. `--archive FILE` uses
an existing candidate archive and its adjacent `SHA256SUMS`. `--worker-image`
can select an existing matching worker; an existing different development tag
is never replaced. Normal runs use three cold/warm pairs per transport;
`--repeats 1` runs one pair each as a shorter smoke test.

The application image inherits the worker's pinned PHP and Composer, adding
GD and the SQLite CLI for Drupal installation. Its local SPX derivative is
built by the actual `performance setup --apply` command. This is test runtime
preparation, not a worker publication or a change to a user's application.

## Coverage

- Fresh installation and replacement through the shell installer, including
  embedded CLI license notices.
- Repeat `init`, shared skills, three completion-hook adapters and Git
  pre-commit; existing project config remains unchanged on repeat.
- Actual Drupal version discovery and configuration, doctor invocation,
  full analysis through 14 checkers, and PHPCS fixes. Findings are allowed;
  incomplete execution or a checker failure to run fails the test.
- All three Stop protocols block on a known `var_dump` finding. Git
  pre-commit rejects that source and accepts its corrected version.
- A database-backed HTTP controller and full-bootstrap Drush command,
  unprofiled requests, incorrect-key requests, and repeated cold/warm captures.
- Five native MCP tools launched from generated project client settings;
  counts, callers, graph edges, source definitions and time units are checked.
- Redaction, path traversal, truncated bodies and malformed metadata.
- SPX source archive SHA256, retained license/build script, applied teardown,
  extension unloading, preserved recordings and a clean fixture Git checkout.

Cold means after `drush cache:rebuild`; OS page cache and OPcache are not
flushed. Timing output is evidence from the fixture under profiling, not a
stable benchmark or a pass/fail speed threshold. Interactive agent clients,
MariaDB/PostgreSQL, DDEV and bare Docker lifecycle coverage remain separate.
Existing local DDEV/bare Docker validation is documented in
[`../spx/RUNTIME.md`](../spx/RUNTIME.md).

## CI

**Actions → Sandbox → Run workflow** selects Drupal 10, Drupal 11 or both.
The manual workflow runs the versions as independent matrix jobs and builds
the matching worker from the selected commit. **Build** also runs both
sandboxes after Docker/Compose fixtures and before image publication. Ordinary
push/PR **Checks** retains its faster native and fixture tests.

Failed assertions fail the job. Application and installer containers are
removed in cleanup. Captures and test images stay local; no global Docker
pruning or personal client configuration is used. Summary, command logs,
capture identities and the MCP transcript are uploaded even on failure,
without uploading the private key, project settings, database or runtime
receipt. Artifact retention is seven days.

## TODO: installation and environment matrix

Installation method and application environment are independent axes. Extend
the shared Drupal 10/11 scenarios with representative combinations:

- [x] Standalone installer with a Compose application and SQLite, verified locally.
- [x] Separate fixture tests for standalone, Composer/npm packages and Compose wrappers.
- [ ] Run Composer/npm install, repeat, upgrade/rollback and wrapper-hook checks
  against fully installed Drupal sites.
- [ ] Run the Compose installation wrapper against the installed sites,
  including check/fix/pre-commit and unchanged application services.
- [ ] Automate isolated DDEV installation and SPX lifecycle in CI.
- [ ] Automate bare Docker launch-argument activation and restoration in CI.
- [ ] Run Drupal on host PHP with the usual Docker-based PHPWarden worker.
- [ ] Run Docker-less CLI installation and recorded-profile CLI/MCP smoke tests;
  verify that `check` clearly reports the missing Docker dependency.
- [ ] Share preservation, failure and cleanup assertions across environments,
  and label evidence by Drupal version, installation method and environment.
- [ ] Run the new GitHub Sandbox workflow after commit/push and inspect its artifacts.

Ordinary checks still require Docker. Native host-PHP SPX installation is not
currently supported and is outside this coverage extension. Live agent-client
sessions and additional database engines remain separate work.

## Update fixtures

Update a fixture by changing its explicit package versions and regenerating
its Composer lock in the same PHP runtime. Never use `composer update` during
the acceptance run. The official release pages document the pinned
[Drupal 10.6.0](https://www.drupal.org/project/drupal/releases/10.6.0) and
[Drupal 11.4.1](https://www.drupal.org/project/drupal/releases/11.4.1) versions.
