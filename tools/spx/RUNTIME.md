# Local runtime adapter validation

The experimental lifecycle is `performance setup`, `status` and `teardown`.
Setup without `--apply` writes only new local integration artifacts. `--apply`
builds/activates DDEV or one Compose service; bare Docker image activation is
manual. The integration is independent of framework profiles and static checks.

## Tested on 2026-10-27

| Runtime | Capture | Activation and restoration |
| --- | --- | --- |
| DDEV 1.25.4, PHP 8.3, Debian/glibc | Nginx/FPM HTTP and CLI | Actual generated Dockerfile and Compose fragment, keyed loopback HTTP, module unloaded after applied teardown |
| Compose 5.6.1, official PHP 8.2, Debian/glibc | HTTP and CLI | Local derivative built, only PHP selected, sibling service preserved, module unloaded after applied teardown |
| Bare Docker, official PHP 8.2 | HTTP and CLI | Image built without changing the original container, known fixture launch manually recreated and then restored |
| Compose, PHP 8.3 Alpine/musl, UID 10001 | Drupal-component HTTP and Drush bootstrap NONE | Sanitized Drupal 11.4.1 mounted read-only, private INI readable by non-root PHP, original image restored |

The generic captures contain 24 calls to the controlled expensive function;
their definitions map to `scenario.php`. The Drupal HTTP capture has 44,121
calls and the Drush capture 141,492 calls. These exercise installation and
reading, not a database-backed Drupal site or performance comparisons. Captures
were gzip; native Zstandard generation, ARM64 and ZTS remain unverified.

Drush exposed a producer detail absent from the small fixtures: CPU overhead
correction can regress a cumulative CPU counter (observed delta: -20 ns).
The reader retains strict wall-clock and structural/count validation, omits
unreliable CPU aggregates and supplies `cpu_omitted_reason`. It does not clamp
CPU data or relabel an invalid wall-clock stream as complete.

Real projects `Forms`, the monorepo and `Contribution/drupal10` were not changed
or started. The last has DDEV/PHP 8.3 but is not a Git checkout, so it is unsuitable
for verifying the Git-exclusion contract directly. DDEV tests used separate
global state with router/SSH/database containers omitted; the existing router's
ID and start timestamp remained unchanged.

## Repeat focused checks

```sh
go test -race ./internal/perfruntime ./internal/performance ./internal/cli
sh -n internal/perfruntime/build.sh
```

Go lifecycle tests use real temporary Git checkouts, linked worktrees and nested
projects, with injected Docker responses. They cover idempotent preparation,
active-runtime reuse, selected-service arguments, interrupted restoration,
changed inputs/owned files, symlinks, tracked-path conflicts, source checksum
failure, secret omission, cancellation and absent status without external tools.
They never launch Docker or download SPX. Reader tests cover both compression
codecs and CPU-counter correction as well as the original stream/path guards.

For runtime acceptance, create a disposable Git project containing
`scenario.php` from `fixtures/`, use a separate private `--state-dir` and the
archive from the bundled source pin. Exercise preparation, applied setup,
status, an unprofiled request, explicitly profiled HTTP and CLI calls, reading,
applied teardown and a final `extension_loaded('spx')` check. Capture the initial
Git status/diff and sibling-container IDs and compare them afterward. Repeat
setup/teardown to check idempotence. Use a fresh project name for every test.

For DDEV, use an isolated `DDEV_XDG_CONFIG_HOME` and omit global router/SSH
containers before starting the fixture. Do not restart the user's global router
or use `ddev poweroff`, global Docker pruning or unscoped container cleanup.
For bare Docker, retain the original fixture container and its known launch
arguments for restoration instead of reconstructing an unknown application.

## Artifact ownership and access

The cache holds one private directory per canonical checkout path. SPX source
is verified before any project adapter is created. The build context excludes
receipts, configuration, keys and recordings. The image retains the SPX license
and producer marker, and restores the base image's configured user.

The private directory is mode 0700. Its reports leaf permits container PHP users
to write without requiring matching host UIDs; the outer directory keeps it
private on the host. The mounted INI is mode 0644 within that private directory
so non-root PHP can load it; the separately exposed key file is mode 0600.
Only the selected service receives those mounts. Raw environment values are
not copied to the Dockerfile, overlay, receipt or command report.

Teardown retains recordings, receipt and images. It validates all owned files
before removal, preserves unrelated Git exclusion bytes/permissions, and can
resume a failed runtime restoration. Removing files alone does not prove that a
running PHP process has unloaded its extension.
