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

Select DDEV with `--environment ddev`; the default is `compose`. Both support
the same installation-method selection and installed Drupal fixtures:

```sh
python3 tools/sandbox/run.py --drupal 10 --environment ddev --output /tmp/phpwarden-ddev-d10
python3 tools/sandbox/run.py --drupal 11 --environment ddev --output /tmp/phpwarden-ddev-d11
```

DDEV tests need DDEV, mkcert and Docker locally. CI uses the checksum-pinned
[DDEV 1.25.4 archive](https://github.com/ddev/ddev/releases/expanded_assets/v1.25.4),
installed into a private tools directory with `sh tools/sandbox/install-ddev.sh DIRECTORY`.
That script extracts tools without sudo, changing system binaries or installing
a host CA. The runtime uses private DDEV global configuration, a private web
cache, no router/SSH agent/database containers, no Mutagen and no settings-file
management. SQLite stays in the sandbox's data directory. DDEV serves the site
through Nginx/FPM; requests run over loopback inside its web container.

Shared fixture provisioning installs locked dependencies and the database with
the PHP 8.3 worker. DDEV then mounts that site under `/var/www/html`; a cache
rebuild clears old container paths before HTTP checks. `init` discovers the
runtime from `.ddev/config.yaml`. Application PHP/SQLite and DDEV versions are
recorded separately from worker versions. DDEV's generated runtime adapters
are locally Git-excluded and removed during teardown. Existing router/SSH-agent
IDs/start times, personal DDEV settings, `/etc/hosts`, fixture settings and
database configuration must remain unchanged. Cleanup stops/unlists only the
unique fixture project; it never calls global `ddev poweroff` or prunes Docker.

The output directory must not exist. By default the suite tests standalone,
Composer, npm and the Compose wrapper in sequence on each installed site.
It builds a development CLI archive for the real `install.sh`, plus two versioned development packages
for package-manager upgrade and rollback. They use the current source with
different version identities, not two historical implementations. Packages
retain source/development identity and require the matching preloaded worker;
no fake published image manifest is supplied.

Use `--installation standalone|composer|npm|compose|all` to select methods. The npm
method requires host Node/npm (CI pins Node 22.22.3); Composer runs in the
sandbox's pinned PHP application image. `--archive FILE` uses an existing
candidate archive and its adjacent `SHA256SUMS`, selecting standalone only.
`--worker-image` can select an existing matching worker; an existing different development tag
is never replaced. Normal runs use three cold/warm pairs per transport;
`--repeats 1` runs one pair each as a shorter smoke test.

The application image inherits the worker's pinned PHP and Composer, adding
GD and the SQLite CLI for Drupal installation. Its local SPX derivative is
built by the actual `performance setup --apply` command. This is test runtime
preparation, not a worker publication or a change to a user's application.

The Compose installation scenario copies the development wrapper templates and
pins a local image ID. Its private test image inherits the selected worker's
tools and adds the current CLI built with container identity. No published
digest is invented, and existing worker tags are not replaced. The application
starts before wrapper checks; its container ID, start time, mounts, configuration,
HTTP response and database configuration hash must survive those checks.

## Coverage

- Fresh installation and replacement through the shell installer, including
  embedded CLI license notices.
- Composer/npm installation, reinstall, upgrade and rollback on each complete
  Drupal site, real check/fix and correct package-wrapper paths in hooks.
  Composer operations preserve existing locked Drupal dependencies and their
  installer paths. npm preserves an existing frontend dependency and settings;
  install scripts stay disabled. Package-managed self-update is refused.
- Compose wrapper repeat init, 14-checker full analysis, an actual PHPCS edit
  and host pre-commit success/failure, including staged/working-tree mismatch.
  Runtime mismatch, invalid image references and unsupported host-only commands
  fail explicitly. Foreign hooks, skills, application Compose files, `.env`,
  Composer dependencies and file ownership are preserved.
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

An incorrect SPX key must create no recording. With DDEV's display-errors setting,
the negative request may include an upstream SPX notice before the body; only
that control permits a non-JSON response. Unprofiled and authorized requests
must still return the complete expected Drupal JSON. A failing late preservation
assertion clears success in the saved summary.

Between installation methods the suite removes only its byte-verified fixture
pre-commit hook and installs the next method's hook. This tests each method;
it does not claim automatic migration of an existing standalone hook to a
different executable. Within each method, reinstall/upgrade/rollback keep the
same hook and project config. The Compose wrapper supports Git pre-commit;
completion hooks, doctor, update and performance commands require a host CLI.
After Compose checks, the last selected host installation runs SPX/MCP; a
Compose-only run installs a host CLI for that separate lifecycle. Summary
evidence identifies the environment and each method. Quality and application
containers use separate Compose project names and cleanup.

Cold means after `drush cache:rebuild`; OS page cache and OPcache are not
flushed. Timing output is evidence from the fixture under profiling, not a
stable benchmark or a pass/fail speed threshold. Interactive agent clients,
MariaDB/PostgreSQL and bare Docker lifecycle coverage remain separate.
Earlier focused DDEV/bare Docker validation is documented in
[`../spx/RUNTIME.md`](../spx/RUNTIME.md).

## CI

**Actions → Sandbox → Run workflow** selects Drupal 10, Drupal 11 or both,
Compose, DDEV or both, and one installation method or all four.
The manual workflow runs version/environment pairs as independent jobs and builds
the matching worker from the selected commit. **Build** also runs both
sandboxes in both environments after Docker/Compose fixtures and before image publication. Ordinary
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
- [x] Run Composer/npm install, repeat, upgrade/rollback and wrapper-hook checks
  against fully installed Drupal sites.
- [x] Run the Compose installation wrapper against the installed sites,
  including check/fix/pre-commit and unchanged application services.
- [x] Automate isolated DDEV installation and SPX lifecycle in CI.
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
its Composer lock in the same PHP runtime. Application dependencies are never
updated during acceptance; only PHPWarden's candidate package is targeted by
Composer updates. The official release pages document the pinned
[Drupal 10.6.0](https://www.drupal.org/project/drupal/releases/10.6.0) and
[Drupal 11.4.1](https://www.drupal.org/project/drupal/releases/11.4.1) versions.
