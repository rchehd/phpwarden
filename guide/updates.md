# Diagnostics and updates

### Diagnose setup and plan updates

```sh
phpwarden doctor
phpwarden doctor --format json
phpwarden updates
phpwarden updates --security --no-dev
phpwarden updates --direct --runtime-version 8.3.26 --format json
phpwarden updates --offline
```

`doctor` reads project configuration, Git access, Composer/Drupal dependency metadata, explicit native config paths and local worker compatibility. JSON/YAML configs receive syntax checks; other native configs are checked for readability. Semantic validation still requires their analyzer. It runs no analyzers, installs no dependencies and pulls/builds no images. Setup errors return exit 3; warnings alone return 0.

`updates` queries available releases for locked Composer packages, including Drupal core/modules managed by Composer, and separately reports security advisories and abandoned packages. Ignored Composer advisories remain visible with an explicit marker. `--security` selects packages with advisories, `--direct` selects root dependencies, and `--no-dev` excludes development packages. These filters combine. Release candidates are queried independently of the worker's PHP and extensions. Their compatibility with the application's runtime and a full dependency upgrade remains unverified; the upgrade skill investigates blockers before proposing changes.

The Composer query runs in a matching local worker with network access and only copies of `composer.json` and `composer.lock`. Project scripts/plugins are disabled; installed project code and host Composer credentials are not mounted. Private repositories may therefore be unavailable. Path repositories are removed from the query copy and their release coverage is marked incomplete; locked path packages remain included in the advisory query.

The Drupal profile also reads the official [Update Status release history](https://www.drupal.org/drupalorg/docs/apis/update-status-xml) directly from the host CLI. Its separate `projects` JSON section reports `supported`, `unsupported`, `insecure`, `revoked` or `unknown`, with release links and security-policy coverage when available. It combines Drupal packages from the lock with packaged `project`/`version` metadata in `.info.yml` and the core version declaration. Packaged projects outside Composer receive the same release-status assessment, including projects with no Composer files at all; this path needs no worker. Present code does not establish which modules are enabled or deployed.

An absent exact version is `unknown`, never inferred revoked. Development snapshots, unreadable metadata, missing feeds and unknown versions make coverage incomplete. Custom code without a Drupal.org project identifier is explicitly outside release-status coverage. Security-policy coverage alone does not establish a vulnerability. `--security` includes insecure/revoked/unsupported releases and known lack of policy coverage; `--no-dev` preserves Composer's development classification. `--direct` requires a known direct Composer relationship, except for core itself, and excludes unmanaged extensions. Drupal feeds share the metadata cache and offline policy; requests use public project identifiers only, with no site URL or UUID. Profile release status does not perform a dependency solve or replace Composer advisories.

PHP release/support data comes from [php.net releases](https://www.php.net/releases/) and the [supported versions table](https://www.php.net/supported-versions.php). The configured PHP branch selects the feed. The application's deployed patch version remains unknown unless supplied with `--runtime-version X.Y.Z`; neither Composer platform nor the worker establishes it. A security-tagged upstream release does not establish which CVEs affect the current runtime. Distributor backports need separate verification.

```yaml
updates:
  timeout_seconds: 120
  max_age_hours: 24
  max_stale_hours: 168
  offline: false
```

Metadata is stored in a private user cache, keyed by source and Composer input/toolset identity. Fresh cached data avoids network and worker startup; offline mode never fetches. A failed refresh can use dated stale data up to `max_stale_hours` but returns incomplete status. Missing sources, malformed metadata, changed inputs and incomplete repository coverage return exit 3, never a clean claim that everything is current. Complete assessments return 0, even when updates are available; invalid input returns 2. Text and JSON are supported. `updates` assesses project dependencies; it does not update the PHPWarden executable.
