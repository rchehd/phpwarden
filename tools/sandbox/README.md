# Installed Drupal sandboxes

This suite installs real Drupal 10.6.0 and 11.4.1 sites from committed Composer
locks, each with its own SQLite database and a controlled custom module. It
complements checker fixtures and package-manager installation tests; it tests
the application lifecycle that those smaller fixtures do not exercise.

Both fixtures use PHP 8.3 on Linux amd64. The version pins are compatibility
test cases, not production version recommendations. The local fixture-only
Composer advisory override allows reproducing historical versions; it does
not disable PHPWarden security checks. Dependency installation needs network
access. Container applications run without network or exposed host ports;
the host PHP application listens only on an ephemeral loopback port.
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

Select bare Docker with `--environment docker`:

```sh
python3 tools/sandbox/run.py --drupal 10 --environment docker --output /tmp/phpwarden-docker-d10
python3 tools/sandbox/run.py --drupal 11 --environment docker --output /tmp/phpwarden-docker-d11
```

This runtime uses a known `docker run` recipe, a read-only project mount,
writable SQLite/public-files mounts, the host UID/GID and no network or host
ports. The standalone installation case needs no Compose application. Selecting
the Compose installation method still exercises that separate quality wrapper.

`performance setup --apply` must build the SPX image and return launch arguments
while the original container keeps running unchanged. The harness then explicitly
stops and renames its own original, starts the known recipe with only the returned
image/report/INI additions, and verifies status and repeat setup. It rejects
unexpected launch flags/mounts and checks that `teardown --apply` refuses automatic
bare-container recreation. After capture, the harness removes the profiled
container, renames/restarts the retained original, verifies its exact ID, image,
configuration and mounts, then removes adapters without `--apply`. A controlled
sibling must retain its ID/start time throughout. Cleanup removes only UUID-owned
fixture containers. `docker-launch.json` records the base argument vector and
checksum, original container ID and restoration result, without the private key.

Select host PHP with `--environment host`:

```sh
python3 tools/sandbox/run.py --drupal 10 --environment host --host-php /usr/bin/php8.3 --output /tmp/phpwarden-host-d10
python3 tools/sandbox/run.py --drupal 11 --environment host --host-php /usr/bin/php8.3 --output /tmp/phpwarden-host-d11
```

This installs and serves Drupal with native PHP 8.3 and SQLite. Docker is still
required for the quality worker; no application container is built or started.
The harness creates its own `php.ini`, empty scan directory, temporary/session
directories, Composer home/cache and PHP wrapper under the output directory.
Only the listed Drupal extensions load; existing global INI files are hashed and
must remain unchanged. Missing shared extensions may be supplied from a private
directory with `--host-extensions DIRECTORY`; the harness never installs or
enables system modules. Composer 2.10.3 is downloaded privately and SHA256-checked.

All four installation methods exercise the same native site. HTTP, Drush,
database configuration, settings and the server PID must survive quality checks.
The Compose wrapper also preserves a foreign, unused Compose fixture; it never
starts that service. Cleanup terminates only the owned PHP server. Host PHP live
SPX setup is unsupported, so this environment explicitly records zero profiles
and skips capture/MCP lifecycle assertions. Recorded-profile CLI/MCP coverage
belongs to the separate Docker-less scenario.

Run the Docker-less scenario separately:

```sh
python3 tools/sandbox/dockerless.py --output /tmp/phpwarden-dockerless
```

Go builds the candidate archive before testing. With `--archive FILE`, the suite
uses an existing archive and adjacent `SHA256SUMS` instead. The offline installer
runs with an allowlist of shell/Git helpers; Docker, Podman, PHP, Composer, npm,
Node, Go and download tools are absent from its `PATH`. The harness creates a
small Git fixture from the Drupal 10 lock and a custom module; it does not claim
to install or run Drupal. CLI installation/reinstallation, checksum failure
preservation, help/version/licenses and repeat `init` must work without those
runtimes. `check` must exit 3 with `complete: false`, a missing-Docker diagnostic,
no findings and no executed checks; text output must say `NOT RUN`.

Recorded-profile CLI and MCP commands then run with an entirely empty `PATH`.
The committed SPX `repeated` and `cached` recordings provide known call counts
and source definitions. All five MCP tools, timing units, source mapping,
redaction, traversal rejection and incomplete capture rejection are exercised.
Project files and recordings must remain unchanged; MCP processes must exit
cleanly. No runtime setup, new captures, global client settings or Docker daemon
operations are involved. Summary, readable/JSON reports, the MCP transcript and
command logs stay in the output directory.

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
sandbox's pinned PHP application image, or private native PHP for `host`. `--archive FILE` uses an existing
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
- SPX source archive SHA256, retained license/build script, runtime restoration,
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
containers use separate lifecycle controls and cleanup. Compose quality services
have their own project name even when the application runs in bare Docker.

Cold means after `drush cache:rebuild`; OS page cache and OPcache are not
flushed. Timing output is evidence from the fixture under profiling, not a
stable benchmark or a pass/fail speed threshold. Interactive agent clients,
MariaDB/PostgreSQL coverage remains separate.
Earlier focused DDEV/bare Docker validation is documented in
[`../spx/RUNTIME.md`](../spx/RUNTIME.md).

## CI

**Actions → Sandbox → Run workflow** selects Drupal 10, Drupal 11 or both,
Compose, DDEV, bare Docker, host PHP or all four, and one installation method or all four.
The manual workflow runs version/environment pairs as independent jobs and builds
the matching worker from the selected commit. **Build** also runs both
sandboxes in all four environments after Docker/Compose fixtures and before image publication. Ordinary
push/PR **Checks** retains its faster native and fixture tests.

Selecting `dockerless` runs only the independent native smoke job; selecting
`all` includes it once alongside the Drupal jobs. Drupal version and installation
method selections apply to the Drupal jobs; Docker-less always tests the offline
standalone installer and recorded readers. **Build** exercises its actual CLI
archive with the same Docker-less harness before uploading the CLI artifact or
publishing a CLI release. Image-only builds retain their application scenarios.

Failed assertions fail the job. Application and installer containers are
removed in cleanup. Captures and test images stay local; no global Docker
pruning or personal client configuration is used. Summary, command logs,
capture identities, bare Docker launch evidence and the MCP transcript are uploaded even on failure,
without uploading the private key, project settings, database or runtime
receipt. Artifact retention is seven days.

## Scenario evidence

The shared harness records each installation and runtime phase separately:

```text
summary.json
evidence/index.json
evidence/11/compose/standalone/install/result.json
evidence/11/compose/composer/install/result.json
evidence/11/compose/npm/install/result.json
evidence/11/compose/compose/install/result.json
evidence/11/compose/shared/capture/result.json
evidence/11/compose/shared/cleanup-compose/result.json
```

Each case has its own `logs/` directory with globally numbered stdout/stderr
files, command exit codes and timeout markers. Partial output survives command
timeouts. The index identifies Drupal version, application environment,
installation method and scenario; `native/dockerless` identifies the independent
recorded-reader suite. Runtime capture/MCP/teardown and application cleanup are
shared phases, not claims of a separate profiler installation for each package.

All application environments reuse the init/check/fix/hook scenario. Shared
preservation guards compare file contents, ownership, modes, absence and symlink
targets during repeat operations and expected failures. Package-specific lock
and dependency assertions remain with their package manager. Cleanup attempts
each remaining owned-resource step even when an earlier step fails. Any failed
case or cleanup keeps the run unsuccessful, including after successful installs.

Container preservation compares identity, configuration, restart state and every
mount property. Mount order is normalized because `docker inspect` may reorder
the same mounts between requests; command and environment order stay significant.

CI uploads this tree on success or failure and renders an installation/scenario
table in the job summary. Manual Drupal artifact names also include the selected
installation method. Inspect local evidence with:

```sh
python3 tools/sandbox/report.py /tmp/phpwarden-sandbox-d11 /tmp/phpwarden-dockerless
```

The report command exits nonzero for failed, missing or invalid evidence.
It summarizes existing results; it does not execute a sandbox. Fast push/PR
Checks still run unit/fixture validation; installed application scenarios remain
in manual Sandbox and Build gates.

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
- [x] Automate bare Docker launch-argument activation and restoration in CI.
- [x] Run Drupal on host PHP with the usual Docker-based PHPWarden worker.
- [x] Run Docker-less CLI installation and recorded-profile CLI/MCP smoke tests;
  verify that `check` clearly reports the missing Docker dependency.
- [x] Share preservation, failure and cleanup assertions across environments,
  and label evidence by Drupal version, installation method and environment.
- [x] Run the GitHub Sandbox workflow and inspect success/failure artifacts:
  [run 40716870828](https://github.com/rchehd/phpwarden/actions/runs/40716870828)
  passed all nine jobs on commit `f5ad3d7`; the earlier failed run supplied regression evidence.

Ordinary checks still require Docker. Native host-PHP SPX installation is not
currently supported and is outside this coverage extension. Live agent-client
sessions and additional database engines remain separate work.

### Project PHPUnit runner

Compose application sandboxes with standalone, Compose or all installation methods
also test `phpwarden test` on a separate copy with locked `drupal/core-dev` dependencies.
Drupal 10 uses PHPUnit 9 and Drupal 11 uses PHPUnit 11. The cases cover passing tests,
name filtering, no matching tests, failed assertions, test exceptions, missing
bootstrap and missing PHPUnit. A test writes a file inside its writable workspace;
the source copy and the running Drupal application must remain unchanged.

`tools/sandbox/drupal10-tests` and `drupal11-tests` contain the separate Composer
locks, and `unit-fixture/ProbeTest.php` supplies the test cases. The original
application fixtures keep their production-only dependency installation.
JUnit, JSON and PHPUnit output are retained under the `project-tests` case's evidence.
The new v28 scenarios have local verification; a GitHub Sandbox run is still pending.

## Update fixtures

Update a fixture by changing its explicit package versions and regenerating
its Composer lock in the same PHP runtime. Application dependencies are never
updated during acceptance; only PHPWarden's candidate package is targeted by
Composer updates. The official release pages document the pinned
[Drupal 10.6.0](https://www.drupal.org/project/drupal/releases/10.6.0) and
[Drupal 11.4.1](https://www.drupal.org/project/drupal/releases/11.4.1) versions.

The `project-tests` case also checks PCOV line coverage and method CRAP on the
locked PHPUnit 9/11 fixtures, union across two test files, HTML/XML artifacts,
and opt-in thresholds without modifying the original project.
