# PHPWarden

PHPWarden runs static quality checks for PHP projects. Profiles provide the conventions, source layout and default rules for each framework or CMS. By default it checks only the lines you changed against a base branch. The tools run inside a Docker image, so the host needs no PHP or Composer. Results come as readable text or JSON, with stable exit codes for CI and scripts.

Drupal is currently the only available profile. Symfony and Laravel profiles are planned; WordPress is a possible future addition. The configuration and examples below use the Drupal profile.

## Requirements

- Linux amd64, Git, and a local Docker daemon.
- Go 1.26.5 only when building the CLI from source.
- A project supported by an available profile. The current Drupal profile supports PHP 8.1, 8.2, 8.3, and 8.4.

## Install

Download `install.sh`, the Linux amd64 archive and `SHA256SUMS` from the same Actions artifact. Install without a release, network access, PHP, Go or Docker:

```sh
sh install.sh --archive ./phpwarden-linux-amd64.tar.gz
```

The default destination is `~/.local/bin/phpwarden`. Use `--dir PATH` for another writable directory, `--checksums FILE` for a checksum file elsewhere, or `--version VERSION` to require the archive's exact version. The installer checks the archive SHA256, executable identity and worker manifest before installing atomically. Repeating the same installation preserves the executable. Replacing a different file requires `--force`; symlinks and package-managed directories are refused. It never changes project settings, hooks or shell startup files.

For a published [GitHub Release](https://github.com/rchehd/phpwarden/releases), download its `install.sh` and select a version explicitly:

```sh
sh install.sh --version v0.1.0-rc.1 --dir "$HOME/.local/bin"
```

That version is an example and must be published before the download can work. Without `--archive` or `--version`, the installer downloads the latest stable release. A local source/development archive can also be installed, but its checks require matching preloaded worker images; it does not gain release automatic downloads or self-update support. Installation currently supports Linux amd64 with GNU shell utilities, tar/gzip, SHA256 tools and `flock`; online installation additionally needs curl. No sudo is invoked. An already installed standalone CLI can use `phpwarden update`.

Manual installation remains available. Download a Linux amd64 archive and `SHA256SUMS`, verify the checksum before extracting, then copy the executable into a writable directory:

```sh
sha256sum --ignore-missing -c SHA256SUMS
tar -xzf phpwarden-linux-amd64.tar.gz
install -Dm755 phpwarden "$HOME/.local/bin/phpwarden"
```

Keep `~/.local/bin` in `PATH`. From the project, run `phpwarden init` and `phpwarden check`. The published CLI embeds its matching compatibility manifest and downloads only the required worker image on first use. It verifies the exact repository digest, platform, protocol and toolset before starting checks. Existing compatible images need no network access. Concurrent requests share a download lock; cancellation or download failure stops the check. `doctor` and `resolve` never download images.

For offline/preloaded installations, set `runner.image_pull: never`; the default is `missing`. The configured `runner.timeout_seconds` also bounds initial image download, so increase it for a slow first download. This setting controls worker image downloads; Composer advisory/update queries have their own offline settings.

### Prepare an image for CI or offline checks

```sh
phpwarden prepare
phpwarden prepare --project /path/to/project --format json
```

`prepare` reads the project config and compatibility manifest, then ensures the selected worker image is present and compatible. A published CLI downloads only the missing digest-pinned image; a compatible local image is reused without registry access. It checks the platform, PHP version, protocol, toolset, capabilities and analyzer versions. Concurrent preparation/check commands share the same download lock. No worker container, analyzer, Composer query or Docker build is started, and Git diff/base refs are not resolved.

This is an explicit preparation command: it permits downloading a missing published image even when `runner.image_pull: never`. It does not change that setting or any project files. Run `prepare` while online, then use `check` with `runner.image_pull: never` to require the preloaded image. The runner timeout also bounds preparation; cancellation returns exit 130. Source/development CLI builds only validate the expected local development image and never pull mutable development tags. Run preparation on the same Docker daemon that will run checks; the host CLI still requires a local Docker Unix socket. Compose chooses its image before the container starts and does not expose a nested `prepare` command.

Preparation can use `composer.lock` to discover compatibility before dependencies are installed, including a monorepo's `composer_root`. Conflicting project/dependency metadata remains an error. Required inputs are configuration and version metadata, not source selection or analyzer setup. JSON reports `ready`, the resolved `selection`, verified local `image_id`, `exit_code` and `diagnostics`. Exit codes are 0 for a ready image, 2 for invalid input/compatibility and 3 for Docker/download/verification failures; these are preparation results, not analysis reports.

This prepares worker images only. For fully offline checks, configure Composer advisories separately with `tools.composer-audit.offline: true` and prepare its advisory cache in an online check first; a cold cache is reported unavailable according to the configured policy. `updates` has its own metadata/offline settings. `doctor` and `resolve` remain read-only and never download images.

### Docker Compose

Published builds with a compatibility manifest also produce `phpwarden-compose-php8.X.tar.gz` for each supported PHP runtime. Download the archive for your project's runtime and `SHA256SUMS`, verify it, then extract it at the project's Git root. It contains `phpwarden`, `.phpwarden.compose.yaml` and `.phpwarden-image`, with the worker pinned by registry digest:

```sh
sha256sum --ignore-missing -c SHA256SUMS
tar -xzf phpwarden-compose-php8.3.tar.gz
./phpwarden init -n --pre-commit
./phpwarden check --commits 2
./phpwarden fix path/to/file.php --only phpcs
```

Commit those three integration files and `.phpwarden.yaml`. The wrapper invokes a separate Compose file and never starts application services. It mounts the project at the same absolute path as on the host, including Git metadata for linked worktrees, and uses the host UID/GID. Run it from inside the project. `check` mounts code read-only with no network; `init` and explicit `fix` use a separate writable service. No Docker socket is mounted. `--composer-root` supports Composer projects inside a monorepo.

Composer auditing first exports only `composer.json`, `composer.lock` and audit settings to a temporary directory. A separate networked service refreshes a shared user cache without mounting project code, vendor or credentials. The offline check then reads that cache and reports missing or stale data according to the project policy. `tools.composer-audit.offline: true` skips network queries; a cold offline cache remains unavailable. Other network commands (`updates`) and agent completion hooks currently require the host CLI. Agent skills and the host pre-commit hook can be installed through Compose. Hooks call the project wrapper, not a container-only executable.

Changing `.phpwarden.yaml` cannot change an existing container's PHP. Replace `.phpwarden-image` with the corresponding digest from the same build's `compatibility.json` when changing runtime; mismatches produce an error. For local development, the image file may contain an immutable local `sha256:<image ID>`. Copyable source templates are in `examples/compose`; generated release archives supply the actual digest. Updating or rolling back Compose means replacing all three integration files together after checking for local edits. Wrapper commands require Linux, Git, Docker Compose and standard shell utilities.

### Composer or npm dependency

Manifest-bearing builds also produce two Linux amd64 project packages. Both contain the exact same CLI as the standalone archive and verify its SHA256 before every launch. They install no analyzer dependencies and run no install scripts. CLI acquisition happens through the package manager; its first `check` downloads the selected digest-pinned worker. Package-managed CLI updates use Composer/npm rather than `phpwarden update`.

The packages are not yet registered on Packagist or npm. Use the generated assets locally or from an exact GitHub Release URL. For npm, a downloaded asset works without a registry package:

```sh
npm install --save-dev --save-exact --ignore-scripts ./phpwarden-npm-linux-amd64.tgz
./node_modules/.bin/phpwarden init -n --pre-commit
npm exec --offline -- phpwarden check --commits 2
```

Keep local package archives in a stable, committed project directory, or use an exact release asset URL, and commit `package-lock.json`. `npm ci --ignore-scripts` reinstalls the locked package. Upgrade or roll back by installing the chosen version's archive and committing the updated manifest and lockfile.

For Composer, add a package repository to the project's existing `repositories` array, using the actual version and asset URL from the chosen build:

```json
{
  "type": "package",
  "package": {
    "name": "rchehd/phpwarden",
    "version": "0.1.0-rc.1",
    "dist": {
      "type": "zip",
      "url": "https://github.com/rchehd/phpwarden/releases/download/v0.1.0-rc.1/phpwarden-composer-linux-amd64.zip"
    },
    "bin": ["bin/phpwarden"]
  }
}
```

The version above illustrates the format; that release must exist before using its URL. For local testing, use an absolute `file:///.../phpwarden-composer-linux-amd64.zip` URL.

Manual snapshots use a Composer version such as `dev-0.0.0-dev.a38dff3`; copy the exact value from the archive's `composer.json` into both the repository and requirement. Stable, alpha, beta and RC versions retain their normal version strings. Other prerelease labels receive the `dev-` prefix because Composer accepts fewer prerelease formats than npm. The embedded CLI keeps its original version and worker manifest.

```sh
composer require --dev rchehd/phpwarden:0.1.0-rc.1 --no-scripts --no-plugins
vendor/bin/phpwarden init -n --pre-commit
vendor/bin/phpwarden check --commits 2
```

Commit `composer.json` and `composer.lock`. `composer install --no-scripts --no-plugins` reinstalls the pin; change the package repository version/URL and requirement together for an upgrade or rollback. Downloaded assets should first be verified against the matching `SHA256SUMS`; the wrapper independently checks its embedded executable. Existing hooks are preserved; new hooks call the installed package wrapper, which remains at a stable project path across package updates.

Package wrappers support host installations with a local Docker daemon. Installing them inside an application container alone does not supply Docker access or map its paths to the host. Use the separate Compose integration for that workflow.

### Build from source

Build the CLI and the Docker images locally. There is one image per PHP version, each with the same checker versions. From this repository:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
docker buildx bake         # all four images
docker buildx bake php83   # only PHP 8.3
```

Instead of building the images, you can pull the development images published by a manual CI run from `main` and give them the local names the CLI expects:

```sh
for v in 8.1 8.2 8.3 8.4; do
  docker pull ghcr.io/rchehd/phpwarden:static-php$v-dev
  docker tag ghcr.io/rchehd/phpwarden:static-php$v-dev phpwarden:static-php$v-dev
done
```

Rebuild or pull again after every update: the CLI rejects an image from another version.

The development toolset revision is defined once in `docker-bake.json` (`PHPWARDEN_TOOLSET`). The CLI embeds that file, development manifest entries derive their IDs from it, and Docker Bake passes the matching ID to the image label. Change this value when worker/toolset compatibility changes, then rebuild the CLI and images together. Published release manifests still specify explicit toolset IDs.

The images are ready-to-run PHPWarden installations: they contain the same CLI binary, the worker, PHP and the analyzers. Their PHP version tags select the execution runtime. Docker's intermediate build stages are not published.

There are two ways to run a check:

| Installation | How it runs |
|---|---|
| Standalone CLI | The host binary reads config, downloads a missing published worker image when needed, and launches it |
| Docker image directly | The image runs `phpwarden worker check` inside the mounted project |

For example, with a Git project configured for PHP 8.4 and its dependencies installed, run from that project's Git root:

```sh
docker run --rm -v "$PWD:/workspace:ro" ghcr.io/rchehd/phpwarden:static-php8.4-dev worker check --commits 2
```

Without command arguments, the image defaults to `worker check`; include `worker check` explicitly when passing options as above. Image-only execution selects the PHP runtime through the image tag; the standalone CLI selects it from `.phpwarden.yaml`.

### CI builds and distribution

The **Checks** workflow runs linters, Go/native tests and fixture checks automatically on branch pushes and pull requests. It can also be run manually without building Docker images or packaging downloads.

The separate **Build** workflow runs manually or on a pushed version tag such as `v0.1.0-rc.1`. It calls Checks for the same commit and builds only after those checks pass. Open the [Build workflow page](https://github.com/rchehd/phpwarden/actions/workflows/build.yml), choose **Run workflow**, select `main`, and choose a target:

| Target | Result |
|---|---|
| `all` (default) | CLI archive and all PHP 8.1–8.4 worker images |
| `cli` | Downloadable CLI archive only |
| `images` | Worker images and their Docker acceptance tests only |

Enable `publish_images` with `all` or `images` on `main` to publish the tested development images to GHCR. Without it, images are built and tested without publication. Version-tag pushes always build both targets, publish version-specific image tags such as `static-php8.4-v0.1.0-rc.1` without moving the floating development tags, and publish a GitHub Release with the CLI archive/checksums/manifest after successful checks and builds. Tags with a prerelease suffix create prereleases; plain `update` selects stable releases.

To request just the CLI archive with GitHub CLI:

```sh
gh workflow run build.yml --repo rchehd/phpwarden --ref main -f target=cli
```

Use `-f target=all -f publish_images=true` to build both targets and publish the images.

Build runs that include the CLI produce a `phpwarden-linux-amd64-<commit>` Actions artifact containing `phpwarden-linux-amd64.tar.gz` and `SHA256SUMS`. Extract the downloaded artifact, run `sha256sum --ignore-missing -c SHA256SUMS`, then extract the tarball. It contains the executable, README, source commit SHA and `release.json` build identity.

With `all` + `publish_images=true`, the image job records real published digests, then the CLI job embeds that manifest. This development snapshot has a `v0.0.0-dev.<commit>` version, supports automatic worker downloads, and can later update to a published stable CLI. The archive includes `compatibility.json`, also uploaded separately. The same artifact also includes checksum-listed Composer/npm packages and four Compose integration archives. CLI-only or unpublished builds produce only the development CLI archive. `internal/toolset/publication-v1.json` declares framework ranges: Drupal 10.0–10.6 on PHP 8.1–8.4 and Drupal 11.0–11.4 on PHP 8.3–8.4. Project Composer requirements can further restrict a combination. The ranges describe declared compatibility, not proof of every application/tool combination.

CLI-only builds and builds without image publication retain the source/development manifest and require locally prepared images. They do not claim release compatibility or support self-replacement. `--version` reports the embedded version and commit; `--build-info` provides JSON including installation type and manifest checksum. GHCR contains worker images, while CLI archives are available through Actions artifacts and versioned GitHub Releases. Manifest-bearing builds also include Compose and Composer/npm packages; those package names are not yet registered in public package registries.

### Update PHPWarden

```sh
phpwarden update --check
phpwarden update
phpwarden update --to v0.1.0-rc.1
phpwarden update --check --format json
```

`update --check` only reads release metadata. `update` downloads the latest stable release for the current platform; `--to` selects an exact published version, including prereleases or an explicit downgrade. An ordinary update never downgrades a newer installed release. Linux amd64 is the initial supported platform. A release must already exist on GitHub; Actions artifacts alone do not provide the update channel.

The updater checks `SHA256SUMS`, the API asset digest when available, archive paths/types/sizes, platform/version/commit, compatibility-manifest identity and the downloaded executable's build metadata. It locks the installation directory, detects concurrent replacement, and atomically replaces the executable only after validation. Download/validation/permission failures preserve the installed CLI. It updates only standalone installations; source builds, containers and known package-manager paths must use their own update mechanism. The installation directory must be writable; the updater does not request elevation.

Project settings, native rules, hooks and skills are preserved. The next `check` downloads its selected worker if missing; `update` does not download all PHP runtimes. Old images remain available for rollback and are not pruned automatically. Updating installed skills/hooks is a separate `init --agents ... --hooks ...` operation; customized skill files still require review. Use `updates` (plural) to assess project dependencies.

## Configure a project

Run `init` anywhere inside the Drupal project:

```sh
phpwarden init
```

It writes an annotated `.phpwarden.yaml` at the root of the Git repository. The file contains every supported setting: detected PHP versions and checker switches are active, while optional parameters are commented out. Its first section looks like this:

```yaml
# Generated by phpwarden init; see README "Configure a project".
version: 1
profile: drupal
php:
  runtime: "8.3"  # from .ddev/config.yaml php_version
  target:
    min: "8.2"  # from composer.json config.platform.php
    max: "8.2"
```

- `php.runtime` is the PHP version the project runs on. It selects the image, for example `8.3` → `phpwarden:static-php8.3-dev`. Init takes it from `.ddev/config.yaml` (and its `config.*.yaml` overrides), then from `config.platform.php` in `composer.json`, and otherwise picks the lowest supported version that every package in `composer.lock` allows.
- `php.target` is the PHP version the code must support, from `8.1` up to the runtime. Init takes it from `config.platform.php`, otherwise it equals the runtime.

If the versions cannot be detected or conflict with `composer.lock`, init stops with exit code 2 and says what to pass, for example `phpwarden init --php-runtime 8.1`. Use `--php-runtime` and `--php-target` to set them yourself. Init never overwrites an existing `.phpwarden.yaml`.

In a terminal, `init` offers agent skills and completion hooks separately, then shows every check with its profile default already selected. Toggle checks by number or ID (for example `2,5` or `rector`), then press Enter to save. `all`, `none`, and `defaults` change the check selection; `q`, Ctrl+C, or closing input cancels before installation. Existing configs are validated and preserved without prompting for checks; agent integrations can still be installed.

For unattended setup, accept the profile defaults with:

```bash
phpwarden init --no-interaction
phpwarden init --no-interaction --pre-commit
```

`-n` is the short form. CI (`CI` set), `--format json`, and redirected input/output also use defaults without prompting. The flag leaves `composer-require-checker` and `composer-unused` disabled. Agent skills require explicit `--agents`; completion hooks require explicit `--hooks`. Every bundled analyzer is already present in the worker image; init records which checks to run.

### Agent skills

Install the shared skills for Codex, Cursor and Claude Code together:

```sh
phpwarden init --agents all --no-interaction
```

Choose specific clients with `--agents codex,cursor` or `--agents claude`; `--agents none` skips installation and does not remove existing skills. In interactive init, press Enter at the agent prompt to skip, or enter `all` or a comma-separated client list.

The installer writes standard Agent Skills packages to `.agents/skills/`. [Codex](https://learn.chatgpt.com/docs/build-skills) and [Cursor](https://cursor.com/docs/skills) discover that directory directly. For [Claude Code](https://code.claude.com/docs/en/skills#choose-where-skills-load), it adds relative directory symlinks under `.claude/skills/` pointing to the same packages. Commit the packages and Claude links to share them with the project. Clients that discover the shared directory can see the skills regardless of which client originally requested installation.

Invoke `$phpwarden-architecture` in Codex or `/phpwarden-architecture` in Cursor and Claude Code. The same invocation convention applies to all four skills:

| Skill | Purpose |
| --- | --- |
| `phpwarden-architecture` | Inspect the project and propose native Deptrac boundaries, then validate the complete graph. |
| `phpwarden-review` | Review the check report and relevant code, prioritizing evidence and security context. Optional category focus keeps the configured checks intact. |
| `phpwarden-doctor` | Explain environment failures using the native `doctor` report and propose concrete setup repairs. |
| `phpwarden-upgrade` | Plan package/PHP upgrades from `updates`, identify dependency blockers, and use upstream Rector sets in dry-run mode. |

Repeated installation reuses identical files and links. Different existing files, conflicting links and symlinked parent directories stop installation without overwriting them. Review customized skill files before updating them. A write failure can leave completed installation files; init reports the failure and a retry can finish it. Skill installation preserves project configuration, harness settings and hooks.

### Agent completion hooks

```sh
phpwarden init --agents all --hooks all --no-interaction
```

`--hooks codex,cursor` selects clients; `--hooks none` skips installation without removing existing hooks. The installer merges its command into `.codex/hooks.json`, `.cursor/hooks.json`, and `.claude/settings.json`, preserving other settings and handlers. Invalid/ambiguous JSON or symlinked settings stop preflight. Its managed handler is updated on repeated init, without adding duplicates. Installation can finish partially after an I/O failure; the error names the incomplete step.

The command contains the local executable's absolute path. Run init on each machine to update that path when the CLI moves. In [Codex](https://learn.chatgpt.com/docs/hooks), review and trust the exact hook via `/hooks`; installing files does not grant that trust. Project trust and hook support in the installed client are required. Client discovery and live completion behavior must be verified in your client version.

At completion, the hook runs the ordinary configured `check` scope and failure policy. Required execution failures also request correction. A clean scoped diff skips the worker; deleted/project files remain part of scope handling. Checks serialize per project. Results are checked afresh; the existing result cache is used only when its normal eligibility and freshness checks allow it. No code, baseline, check setting or dependency is changed automatically.

[Claude Code](https://code.claude.com/docs/en/hooks) and Codex receive a blocking Stop decision; [Cursor](https://cursor.com/docs/hooks) receives a follow-up message. Cursor stop hooks request continuation rather than enforce a commit gate. Aborted/error runs and nested PHPWarden executions are ignored. Automatic correction is bounded to two attempts per user turn by default; unresolved results are reported after that limit. A new user turn starts a new budget. Optional check failures remain nonblocking but are reported as incomplete. Cursor prints nonblocking limitations to the hook's stderr. Multi-root Cursor workspaces need a manually selected `--project PATH` in the hook command.

```yaml
agents:
  completion:
    enabled: true
    max_attempts: 2
    timeout_seconds: 300
```

`max_attempts` accepts 1–5; timeout accepts 1–540 seconds. The installed handler allows 600 seconds for checking and cleanup. Set `enabled: false` to suspend installed completion checks. Git pre-commit remains a separate integration.

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

The Composer query runs in a matching local worker with network access and only copies of `composer.json` and `composer.lock`. Project scripts/plugins are disabled; installed project code and host Composer credentials are not mounted. Private repositories may therefore be unavailable. Path repositories are removed from the query copy and their release coverage is marked incomplete; locked path packages remain included in the advisory query. Packages/modules outside Composer are outside this command's coverage.

PHP release/support data comes from [php.net releases](https://www.php.net/releases/) and the [supported versions table](https://www.php.net/supported-versions.php). The configured PHP branch selects the feed. The application's deployed patch version remains unknown unless supplied with `--runtime-version X.Y.Z`; neither Composer platform nor the worker establishes it. A security-tagged upstream release does not establish which CVEs affect the current runtime. Distributor backports need separate verification.

```yaml
updates:
  timeout_seconds: 120
  max_age_hours: 24
  max_stale_hours: 168
  offline: false
```

Metadata is stored in a private user cache, keyed by source and Composer input/toolset identity. Fresh cached data avoids network and worker startup; offline mode never fetches. A failed refresh can use dated stale data up to `max_stale_hours` but returns incomplete status. Missing sources, malformed metadata, changed inputs and incomplete repository coverage return exit 3, never a clean claim that everything is current. Complete assessments return 0, even when updates are available; invalid input returns 2. Text and JSON are supported. `updates` assesses project dependencies; it does not update the PHPWarden executable.

Set a checker's `enabled` to `true` or `false` to turn it on or off. Uncomment optional settings to customize them; commented paths are examples and must match your project. `scope.include` accepts paths, and `scope.exclude` accepts globs. Per-checker `include` and `exclude` narrow the global scope. For example:

```yaml
scope:
  include: [web/modules/custom/my_module]
  exclude: ["**/generated/**"]
tools:
  phpstan:
    enabled: true
    level: 6
    exclude: ["**/tests/**"]
```

See the [full generated example](examples/drupal.full.phpwarden.yaml) for all settings. If you already have a configuration, copy the blocks you need from that example; rerunning `init` preserves your file. Detailed rules remain in each analyzer's native config, referenced by its `config` setting.

### Git pre-commit hook

To configure a project and install its pre-commit hook:

```sh
phpwarden init --pre-commit
```

This also works when `.phpwarden.yaml` already exists. Repeating the command keeps the configuration and reports whether the hook was newly installed or already present. Plain `init` does not install a hook.

The hook checks whole staged files within the configured source roots and project-file scope, using the project's enabled checks and policies. It does not compare against a remote branch, so it works for a first commit and honors Git's temporary index for `git commit -a`. Blocking findings or a required runner/checker failure abort the commit. Deleted files and staged files outside the configured scope are skipped.

Selected files must match their staged versions. If one also has unstaged edits or was deleted only from the working tree, the hook stops and explains how to resolve the mismatch. It does not stash, fix, stage or rewrite project files. Tool configs, dependencies and cross-file analysis use the current working-tree context. Changes to the index during checking invalidate the result.

Installation uses Git's effective hooks directory, including a project-local `core.hooksPath` and the common hooks directory of linked worktrees. Conflicts with an existing hook are checked before configuration generation; a different hook is preserved. To integrate with an existing hook manager, invoke this command from its pre-commit hook:

```sh
phpwarden hook pre-commit
```

Other shared hook directories outside the project are left for manual integration. The generated hook records the absolute path of the CLI used to install it, so Git clients do not need `phpwarden` on their `PATH`; keep that executable at the recorded location. If moving it, remove the generated PHPWarden hook and install again. Git and Docker must still be available to the checking process. Install hooks per clone; linked worktrees share the default hook directory. Existing project configuration, Docker images and policy settings are reused.

### Monorepo

When the Drupal project is in a subdirectory, init records it as `composer_root`:

```yaml
composer_root: "platform"  # directory with composer.json
```

`composer.json`, `composer.lock`, `vendor`, and the web root are read from there; diffs and reported paths cover the whole repository. Drupal packages from Composer `path` repositories, such as an install profile in `profile/` or modules in `packages/`, are checked at their real location. When the repository has several Drupal projects, run init inside one of them or pass `--composer-root DIR`. PHPCS, PHPMD, PHPStan, and Twig CS Fixer use their configs in the repository first and fall back to the ones in `composer_root`.

That is enough to start. PHPWarden finds the web root and custom code (`modules/custom`, `profiles/custom`, `themes/custom`) from `composer.json` and the directory layout. Core, contrib, and vendor are always excluded. Changes are compared against `origin/HEAD`; set `scope.base` to use another branch.

[examples/drupal.phpwarden.yaml](examples/drupal.phpwarden.yaml) shows the common settings, and [schemas/config-v1.schema.json](schemas/config-v1.schema.json) lists all of them. To check the file and see the final settings with their sources:

```sh
phpwarden config validate
phpwarden config show
```

PHPWarden looks for `.phpwarden.yaml` from the current directory up to the Git root. Use `--project PATH` or `--config PATH` to point elsewhere, for example to a Drupal project inside a monorepo.

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

## Analyzer concurrency

Worker analyzers run sequentially by default. Set the maximum number running at once in the project's configuration:

```yaml
runner:
  concurrency: 4
```

`1` keeps sequential execution; a larger integer sets a limit; `0` runs all requested worker analyzers concurrently. Disabled checkers and checks without applicable files still skip. `init` includes this setting as a commented example. The isolated Composer advisory request performed by the launcher remains a separate phase.

Parallel execution preserves the result ordering, findings, failure policies and input validation. Global and per-checker timeouts still apply; the global timeout includes time waiting for a worker slot. This controls concurrency between analyzers; `tools.syntax.parallel_jobs` separately controls processes inside the optional Parallel Lint backend. More concurrent analyzers can consume more CPU and memory.

Rebuild or pull matching worker images together with the CLI to use this setting; an older worker does not support it.

## Repeated and concurrent runs

Checks use separate temporary directories and analyzer caches for every run. Shared Composer advisories use a private cache, an interprocess lock, and atomic replacement. A cancelled process releases its locks; checker subprocess groups and Docker containers are cleaned up on exit. Fixes on the same project wait for one another without creating lock files in the source tree. Input hashes and the selected Git scope are checked again before accepting a result.

The public `check` command can reuse complete results for selected file-only checks. Enable this explicitly:

```yaml
runner:
  result_cache: true
```

This applies to `debug`, `syntax`, `yaml`, `whitespace`, and `drupal-config`. Composer audit can accompany them: its network/cache freshness policy is evaluated on every invocation. Any other enabled checker bypasses the result cache, because project configs, dependencies, and executable autoloaders need live analysis. Native analyzer caches remain private to each run.

Keys include the canonical project root, file content hashes, raw and effective configuration, discovered project facts, selected files and line ranges, checker selection, toolset, runtime, and immutable local image digest. Entries expire after 24 hours. Incomplete, stale, interrupted, or invalid reports are never reused; complete reports containing findings may be reused. Cache hits receive a new run ID and are validated against the current request. JSON exposes `environment.cache` with `hit`, `miss`, or `bypass` and an optional reason; an unavailable result cache falls back to live analysis.

Use `phpwarden check --no-cache` to force live analysis. `--fix` always prepares changes and rechecks without the result cache. Cache files live under `$XDG_CACHE_HOME/phpwarden` or the OS user cache directory, with private permissions. Existing non-private advisory entries are ignored and refreshed; a non-private cache directory must be removed or made private before caching can resume. Cache locks require local Unix advisory-lock support; unsupported filesystems cause live checks to bypass caching and fixes to stop explicitly.

## Checkers

All checkers except `composer-require-checker` and `composer-unused` are enabled by default in the Drupal profile. `composer-audit` is the only one that uses the network, and it is optional by default.

These two checks require an explicit choice for the project:

| Checker | Why it starts disabled |
|---|---|
| `composer-require-checker`, `composer-unused` | Inspect a whole standalone Composer package in full mode; Drupal installer layouts are unsupported |

Enable one with `tools.<id>.enabled: true`, or select it for a single run using `--only`. `disabled` means configuration turned a checker off; `no files` means it has no applicable files in the selected scope; `no config` means a required native rules file was not found. Enabling a checker does not widen the selected scope or bypass its prerequisites.

| ID | Checks |
|---|---|
| `composer-validate` | `composer.json` and `composer.lock` validity, like `composer validate` |
| `composer-audit` | Known vulnerabilities and abandoned packages in `composer.lock`, like `composer audit` |
| `dead-code` | Unused methods, properties, constants, and enum cases; enabled, advisory findings |
| `debug` | Debug leftovers: PHP calls such as `var_dump`, `print_r` (not with `$return`), Devel's `dpm`/`kint`/`dd`, and `die`; `console.log` and `debugger` in JavaScript (not `*.min.js`); `dump()` and `kint()` in Twig |
| `syntax` | PHP syntax (`php -l`, optional Parallel Lint) in `.php`, `.module`, `.inc`, `.install`, `.theme`, `.profile`, `.engine`, `.test` |
| `yaml` | YAML syntax |
| `whitespace` | Trailing whitespace and unresolved merge conflict markers |
| `drupal-config` | Top-level `uuid` and `_core` keys in module `config/install`, `config/optional`, `config/schema` |
| `eslint` | JavaScript lint using the nearest project config; skipped without a config |
| `eventhorizon` | Drupal caching heuristics and exported config relationships; enabled, advisory |
| `gitleaks` | Potential secrets in selected files; values and source text are redacted |
| `stylelint` | CSS/SCSS/Sass/Less lint using the nearest project config; skipped without a config |
| `jscpd` | Duplicate code across custom source roots, reported as warnings |
| `phpcs` | PHP_CodeSniffer with Drupal Coder (`Drupal`, `DrupalPractice`); PHPCompatibility on request |
| `phpmd` | PHPMD: unused code and code size, reported as warnings |
| `phpstan` | PHPStan with phpstan-drupal and deprecation rules |
| `composer-require-checker` | Undeclared symbols in standalone Composer packages; opt-in, full only |
| `composer-unused` | Dependency removal candidates; opt-in, advisory, full only |
| `deptrac` | Project-defined architecture layers and dependency rules; skips without config |
| `rector` | Drupal/PHP modernization suggestions from an isolated Drupal Rector bundle; enabled, dry-run |
| `semgrep` | Local PHP security search/taint rules using Semgrep CE; enabled in the Drupal profile |
| `twig-cs-fixer` | Twig template style with Twig CS Fixer |

Every checker accepts these settings under `tools.<id>`:

- `enabled`: run the checker.
- `required`: when the checker cannot run, `true` fails the whole check, `false` reports an incomplete result.
- `fail_on`: `error`, `warning`, or `never` (report without failing).
- `timeout_seconds`, `include`, `exclude`.

### PHPCS

PHPCS uses the nearest `.phpcs.xml`, `phpcs.xml`, `.phpcs.xml.dist`, or `phpcs.xml.dist` in the project. Without one it uses `Drupal` and `DrupalPractice`. Other useful settings are `standards`, `sniffs`, `exclude_sniffs`, `standard_paths` for custom standards, and `compatibility.enabled: true` for PHPCompatibility against `php.target`.

### PHPMD

PHPMD reports unused code and oversized classes and methods as warnings, so it does not fail the check; set `fail_on: warning` to block on them. A finding about a method or class shows when its declaration line changes.

PHPMD uses the nearest `phpmd.xml`, `phpmd.xml.dist`, or `.phpmd.xml` in the project, and a `phpmd.baseline.xml` next to it. Paths in the ruleset's `<rule ref>` are relative to the project root. Without a project ruleset it uses `rulesets` (`unusedcode` and `codesize`) without `exclude_rules` (`UnusedFormalParameter`, because Drupal hooks have fixed signatures). Other settings are `config`, `discover_config`, `extra_extensions`, `extra_inputs`, and `memory_limit_mb`. A file PHPMD cannot parse is reported as a warning for the whole file.

### Dead code

`dead-code` is enabled by default in the Drupal profile; run it alone with `phpwarden check --only dead-code` or disable it with `tools.dead-code.enabled: false`. An explicit disable in an existing project config takes precedence over the profile default. It uses PHPStan with [dead-code-detector](https://github.com/shipmonk-rnd/dead-code-detector), so it needs the same installed Composer dependencies and PHPStan settings. A member is dead only if nothing in the project uses it, so it analyses all custom code and reports only members declared in the checked files and lines. Findings are warnings; set `fail_on: warning` to block on them. The checker is required by default, so an execution failure makes the run incomplete and unsuccessful.

Drupal calls much of its code without a PHP call site, so these count as used: constructors, methods with Drupal hook or Drush attributes, `'::method'` and `[$this, 'method']` callbacks in the same class, any method name used as a string in event subscribers and trusted-callback classes, and `Class::method` or `service:method` in the module's routing and services YAML. Other entry points, such as callbacks in another module, can still show up as false positives. Settings are `timeout_seconds` (600) and `memory_limit_mb` (1024).

### ESLint and Stylelint

Node.js is included in the worker image; no host Node.js is needed. Each file uses its nearest project config, falling back to `composer_root` for code outside the Composer project. `config` explicitly selects a project-relative config; `discover_config: false` disables discovery. Without a config the checker reports `skipped: no_config`, with a diagnostic identifying the affected files. No rules are invented for an unconfigured project.

ESLint discovers `eslint.config.js`/`.mjs`/`.cjs`, `.eslintrc` variants, and `eslintConfig` in `package.json`. Stylelint discovers `stylelint.config.js`/`.mjs`/`.cjs`, `.stylelintrc` variants, and `stylelint` in `package.json`. ESLint 8–10 and Stylelint 14–17 project installations take precedence over the bundled ESLint 9.39.5 and Stylelint 17.16.0. ESLint 10 requires flat config. Declared but missing tool dependencies produce `unavailable`; install the project's packages before checking it. Plugins, shared configs, parsers, and SCSS syntax packages must be installed in the project and compatible with the chosen tool.

Rule findings respect changed lines and native severity. Syntax errors apply to the entire selected file. Fixing and native result caches are disabled. ESLint selects `.js`, `.mjs`, `.cjs`, `.jsx`; Stylelint selects `.css`, `.scss`, `.sass`, `.less`. Both support `extra_extensions` and `extra_inputs`; minified files are excluded by default. Imported configs and installed packages participate in stale-input detection.

### Duplicate code

`jscpd` uses the pinned Node engine 4.3.0. It searches all configured custom source roots, applying scope and checker exclusions, without following symlinks. Both sides of a duplicate may be outside the changed files, but a finding is shown only where a duplicate block overlaps selected lines. The reported line is the first selected line inside that block. Findings are warnings; use `fail_on: warning` to block on them.

Defaults are `min_tokens: 70`, `min_lines: 6`, `mode: mild`, and a 300-second timeout. These settings override native thresholds and mode. An explicit `config` or a root/Composer-root `.jscpd.json` can supply formats, extension mappings, ignores, and size limits. Native output paths, reporters, and global percentage thresholds do not control PHPWarden's report or verdict. PHP files with Drupal extensions such as `.module` and `.inc` use the PHP tokenizer. The scan includes eligible files beneath source roots, including files ignored by Git; exclude generated directories with `scope.exclude` or `tools.jscpd.exclude`.

### PHPStan

PHPStan loads the project's Composer autoloader, so run `composer install` first. The installed `drupal/core` must match `composer.lock`. If it does not, PHPStan is reported as unavailable and the other checkers still run.

PHPStan uses `phpstan.neon`, `phpstan.neon.dist`, or `phpstan.dist.neon` from the project root. Without one it analyses at level 3. Other useful settings are `level`, `baseline`, `includes`, `bootstrap_files`, `deprecations`, and `memory_limit_mb`.

#### Strict rules

The bundled `phpstan/phpstan-strict-rules` 2.1.0 is opt-in through `tools.phpstan.strict_rules.enabled: true`. Findings use the normal PHPStan severity, scope, baseline, and failure policy; there is no separate checker ID. `strict_rules.all` defaults to `true`; `strict_rules.rules` overrides individual native switches, such as `disallowedLooseComparison`, `disallowedEmpty`, `booleansInConditions`, and `strictFunctionCalls`. Use `all: false` with selected switches set to `true` to choose rules individually. The schema lists all 19 switches from the pinned package.

Loading this extension also changes core PHPStan parameters. `enabled: true, all: false` keeps those changes even with no named rules selected. `enabled: false` means PHPWarden does not include the package; a native project NEON may still include it explicitly. PHPWarden's strict-rule switches apply only to the normal PHPStan run, not to the separate `dead-code` run. Advanced parameters remain in project NEON or `tools.phpstan.includes`.

### EventHorizon

`eventhorizon` is enabled by default in the Drupal profile, with advisory findings (`fail_on: never`). Disable it with `tools.eventhorizon.enabled: false`, or run it on its own with `phpwarden check --only eventhorizon`. An explicit `enabled: false` in an existing project config retains precedence over the profile default.

It uses the caching and config APIs from [QED42 EventHorizon](https://github.com/qed42/eventhorizon-cli-mode), pinned to 0.1.0 commit `ae11df1cd8e4e2b843c6c53bac60026ef344935f`, with PyYAML 6.0.3. Python and the sources are bundled in the worker; Drupal bootstrap, Composer dependencies, and a site database are not required. The integration includes caching detectors and config relationship checks. General regex security rules and code metrics are outside this adapter.

Caching checks use whole selected PHP/Drupal files, then filter findings to selected lines. Exported config requires explicit project-relative `config_sync` directories. Include those directories in `scope.include` as well as your source roots. When a selected `.yml` file belongs to one of them, all top-level `.yml` files in that directory are analyzed and fingerprinted, including files outside the diff and checker include/exclude filters. Relationship findings describe the directory as a whole and remain visible in a deletion-only diff. Config sync paths are not discovered by executing `settings.php`.

Defaults are `caching: true`, `config_sync: []`, `exclude_rules: []`, `timeout_seconds: 120`, `required: true`, and `fail_on: never`. Findings are warnings or info because these are heuristics; a missing cache marker can be intentional or supplied elsewhere. Use `exclude_rules` for known exceptions, such as `caching.missing_cache_metadata` or `config.orphaned_entity`; the schema lists supported rule IDs. `fail_on: warning` enables blocking. A broken analyzer still produces an error by default; `required: false` permits an explicitly incomplete result.

Analysis uses private copies of inputs. Symlinks, unreadable files, invalid text, malformed/duplicate-key/non-mapping config YAML, analyzer warnings/errors, unexpected output, and inputs totaling over 32 MiB are rejected. No source files or CSV/XLSX reports are written to the project. Accuracy and performance still need validation on real Drupal projects before considering wider use.

### Composer dependency checks

`composer-require-checker` and `composer-unused` are disabled by default. Enable them explicitly, or use `--only composer-require-checker,composer-unused --mode full`. Other modes report `skipped: full_scan_required`. They inspect the whole selected Composer package, including dependencies, regardless of diff or source include patterns. Both default to advisory `fail_on: never`; use `warning` to block on candidates. Neither removes packages or supports `--fix`.

Set each tool's `composer_root` to choose a standalone package; otherwise the global Composer root applies. Composer 2 installed metadata and all direct dependencies must exist; missing dependencies produce `unavailable: dependencies_missing`. The supported layout is `<vendor-dir>/<vendor>/<package>` with regular local files. Drupal installer paths such as `web/core`, virtual/replaced packages and linked dependencies report `skipped: unsupported_layout`. No blanket Drupal exclusions are supplied. In particular, YAML, plugin discovery and dynamically referenced services can make composer-unused candidates false positives: review them before removal.

ComposerRequireChecker is pinned to 4.7.1 on PHP 8.1 and 4.16.1 on PHP 8.2–8.4. Configure native JSON through `config`/`discover_config`, `scan_files` relative to the package root, and `allowed_symbols`. Native JSON accepts `scan-files`, `symbol-whitelist` and `php-core-extensions`. Root autoload sources and direct dependency symbols are analyzed; `autoload-dev` is not included automatically. `composer-unused` 0.9.7 supports native PHP `config`, `exclude_packages` and package-relative `exclude_directories`. Config discovery checks the project root, then the selected Composer root (`composer-require-checker.json` or `composer-unused.php`). PHP config and Composer's `installed.php` are trusted executable inputs inside the worker. List additional config inputs in `extra_inputs`. Both tools support timeout/memory limits and reject incomplete native reports.

### Parallel syntax checking

`tools.syntax.backend: parallel-lint` selects the isolated PHP Parallel Lint 1.4.0 backend; `parallel_jobs` defaults to 4 (range 1–32). It lints exactly the selected Drupal PHP files, without executing source/autoload. Native lint skip directives produce an incomplete result, never a clean pass. File names containing semicolons or line breaks require the default `php` backend. Parallel Lint is optional; no performance advantage has been established. The check ID and rule remain `syntax` / `php.syntax`.

### Deptrac

`deptrac` checks the architecture defined by your project and is enabled by default. Without a discovered configuration it reports `skipped: no_config`, including with `--only`, with guidance for installing and using the architecture skill. A missing explicitly configured file is an error. PHPWarden does not supply default Drupal layers; the optional `phpwarden-architecture` agent skill helps create project-specific rules. An existing `enabled: false` remains effective.

The isolated bundle pins Deptrac 4.4.0 and PHPStan 2.3.1. Version 4.4.0 is the newest Deptrac release compatible with the worker's complete PHP 8.1–8.4 matrix; Deptrac 4.5 and newer require PHP 8.2. All bundle dependencies have a separate lock and manifest identities. The adapter uses the pinned analysis API, disables Deptrac's persistent cache, and keeps generated configuration and temporary files in a private directory. It avoids the CLI analysis command's unlimited-memory override and retains `memory_limit_mb`.

Discovery checks `deptrac.yaml`, `deptrac.yml`, `deptrac.yaml.dist`, and `deptrac.yml.dist`, first at the project root and then at the Composer root. `config` sets an explicit project-relative YAML path. Configure `paths`, `layers`/collectors, `ruleset`, `exclude_files`, `skip_violations`, and analyzer settings under the native `deptrac` key. Graph paths are directories relative to the main native config. Local YAML imports, including baseline files, are supported as strings or `{resource: file.yaml}` entries. Composer collector `composerPath`/`composerLockPath` values are relative to the project working directory.

This adapter accepts self-contained YAML settings and explicit local YAML imports. PHP configs, custom services/parameters/tags, YAML anchors/aliases, remote/glob/optional imports, the experimental `phpstan_parser` flag, and graph paths containing symlinks or unreadable directories are unsupported. Inputs must remain inside the project. `extra_inputs` validates additional local files; project fingerprints also track graph/config/dependency metadata. No project PHP autoloader is explicitly loaded by the adapter.

Deptrac analyzes the complete graph from native `paths`, including files outside the diff. Its file collector scans `.php` files; Drupal classes in `src/` are covered, while procedural `.module`/`.inc` files are not. PHPWarden's `include`/`exclude` filters only findings, preserving graph context. Diff results require the dependency's source line to be selected. Changes to a dependency or architecture rules can affect unchanged callers: use `--mode full` for that review. A config-only diff without selected PHP files is skipped as `no_files`.

Forbidden dependencies are `deptrac.violation` errors. `report_uncovered` exposes `deptrac.uncovered` warnings; `fail_on_uncovered` always exposes them as errors. With `report_skipped`, accepted baseline entries are informational `deptrac.skipped` findings. Ambiguous layer assignments produce global `deptrac.configuration_warning` warnings. `fail_on` controls whether findings block the run; the default is `error`, and `never` makes findings advisory. Parsing errors, invalid rules, stale baseline errors, timeouts, and incomplete reports remain incomplete results regardless of this policy. Native and Docker fixture tests cover this behavior.

### Drupal Rector

`rector` is enabled by default in the Drupal profile and only suggests changes. It uses isolated, locked bundles of Drupal Rector 1.1.3, Rector 2.6.1, and PHPStan 2.3.1. Drupal Rector 1.1.3 explicitly conflicts with Rector 2.6.2 and newer; the lock preserves that constraint. Disable it with `tools.rector.enabled: false`, or run it on its own with `phpwarden check --only rector`. Existing project files with explicit `enabled: false` keep that choice.

With no project config or explicit targets, sets, or rules, the Drupal profile selects the bundled upstream deprecation sets through the current Drupal minor from resolved `framework.version`. For example, Drupal 11.4.10 uses sets through 11.4 and retains 11.4.10 for upstream version guards. Automatic presets support Drupal 10.0–10.6, 11.0–11.4, and 12.0; unknown or newer versions require a project config or an explicit selection. Presets suggest modernization for the current version without selecting a future Drupal upgrade.

An explicit `config` takes precedence over discovered `rector.php` / `rector.dist.php`; discovery checks the project root before the Composer root. Any project config, `target_php`, `target_drupal`, `sets`, or `rules` replaces the automatic preset. Explicit selections can be combined with a project config. `skip_rules` also works with the automatic preset. Upgrade targets are independent of the current project's `php.runtime`, `php.target`, and `framework.version`; they never change image selection or the normal compatibility checks. Without an explicit PHP upgrade target, Rector uses the current PHP target maximum as its feature ceiling and adds no PHP upgrade sets.

The automatic Drupal preset needs Composer-installed package metadata in the selected `composer_root` (including a custom `vendor-dir`). Restore project dependencies with `composer install` before checking. Missing metadata reports `unavailable: dependencies_missing`; PHPWarden does not install dependencies.

With applicable PHP files, an unresolved or unsupported automatic preset produces `error: config_invalid`. With its default `required: true`, this makes the run incomplete with exit code 3. With no applicable files, it reports `skipped: no_files`.

`target_php` enables cumulative PHP sets through the chosen version (8.1–8.5). `target_drupal` selects the pinned minor deprecation sets through the chosen target, including older Drupal sets. Supported targets are 10.0–10.6, 11.0–11.4, and 12.0. This sets Drupal Rector's assumed destination and minimum supported version; suggested code may drop compatibility with older Drupal. The tool's rule coverage is incomplete and a successful check does not prove upgrade readiness. Future minor sets and the upstream opt-in breaking sets are not enabled automatically.

`sets` accepts bundled names such as `drupal-10.3` and `drupal-11.0`, or local PHP set files. `rules` and `skip_rules` accept fully qualified Rector class names. Configure rules needing options, custom rules/autoloading, extra sets, and native skipped paths inside project PHP config. `include`, `exclude`, and `extra_extensions` restrict selected files. `extra_inputs` validates additional local files; project context fingerprints also track metadata for imports, custom rules, and dependencies. Native PHP configs and project autoloaders execute project code, as they do for the other PHP analyzers.

The adapter always passes `--dry-run`, forces selected files and PHP extensions after importing project configuration, disables parallel processing, and puts caches in a private temporary directory. The worker mounts the project read-only. `dry_run` accepts only `true`; `--fix` continues to support PHPCS and Twig CS Fixer only. Suggestions identify affected original lines and the rules used for that file; raw patches are not exposed in the normalized report. Diff mode filters suggestions to selected changed lines; use `--whole-files` or `--mode full` for wider review. Native failures and partial reports produce an incomplete result, even when `fail_on: never`. Suggestions are warnings and advisory by default; `fail_on: warning` makes them fail the check. Native and Docker fixture tests cover this behavior.

### Semgrep CE

`semgrep` is enabled by default in the Drupal profile and runs with bundled rules without a project config. Disable it explicitly with `tools.semgrep.enabled: false`, or run it on its own with `phpwarden check --only semgrep`. Existing configurations that explicitly set `enabled: false` retain that choice, including files generated by older versions of `init`.

The worker includes the pinned Semgrep CE 1.182.0 engine. PHPWarden uses its native protocol directly; it does not install the Python CLI, contact the registry, send telemetry, or use Pro analysis. The bundled ruleset `1.0.0` contains four PHPWarden-authored taint rules for request data reaching shell commands, SQL query text, HTML/trusted markup, and `unserialize`. These rules cover `$_GET`, `$_POST`, `$_REQUEST`, `$_COOKIE`, and request `query`/`request` accessors. They do not treat every Drupal `#markup` value as an XSS vulnerability.

CE taint analysis operates within a function or top-level body. It does not track flows through other functions or files. PHPWarden analyzes whole selected files, including Drupal extensions, and reports matches intersecting selected lines. A newly changed taint source may feed a sink outside the diff; use `phpwarden check --only semgrep --whole-files` or a full scan when reviewing security changes. Findings indicate potential problems; they do not prove exploitability or complete security coverage.

Project rules are added through a root/Composer-root `.semgrep.yml` or `.semgrep.yaml`, an explicit `config`, and additional `rulesets` paths. `use_defaults: false` removes the bundled rules. These are self-contained local PHP search/taint YAML or JSON rules: remote configs, Jsonnet, validators, executable predicates, Pro features, autofixes, and native rule `paths` are unsupported. Use checker `include`/`exclude` for paths. `rules` selects IDs, `exclude_rules` removes IDs, and `severities` selects `ERROR`, `WARNING`, and `INFO`. Unknown/duplicate IDs are errors. CLI `nosemgrep` processing is not implemented by this adapter; use explicit rule exclusions or a baseline.

Defaults are `required: true`, `fail_on: error`, `timeout_seconds: 300`, `rule_timeout_seconds: 5`, and `memory_limit_mb: 1024`. Native ERROR/WARNING/INFO map to error/warning/info. Parse failures, skipped targets/rules, memory/time/fixpoint failures, or incomplete coverage make the checker incomplete; a successful process exit alone is insufficient. Inputs must be regular files without symlinks, at most 16 MiB each. Rules and baseline files are limited to 4 MiB each. Analysis runs on private copies and does not execute project PHP or write fixes. Rule text is used for messages; native source snippets, metavariable values, and traces are omitted from reports.

Semgrep findings expose `fingerprint` in JSON. To accept existing findings, first collect fingerprints from an unsuppressed report into a local JSON file shaped as `{"version":1,"fingerprints":["<64 hex characters>"]}`, then set `tools.semgrep.baseline` to its project-relative path. Fingerprints use the rule ID, original path, matched bytes, and duplicate occurrence, so unrelated line shifts preserve suppression. A baseline suppresses findings only; it cannot suppress analyzer failures. Native and Docker fixtures cover the pinned core and bundled rules.

### Gitleaks

Gitleaks 8.30.1 checks the current contents of selected files, including committed, staged, unstaged, and untracked changes. Diff mode reports only findings whose line ranges overlap selected lines; full and whole-files modes report the corresponding selected files in full. Multiline findings are anchored to the first selected line in the match. This scans files in the working tree, without scanning Git history or the index as a separate snapshot.

The checker is enabled and required by default. Findings are errors; `fail_on: never` makes them advisory. Secret values, matched text, native descriptions, and native process output are omitted from PHPWarden reports and diagnostics; native JSON reports are redacted and removed with the private temporary directory. Redaction is always enabled.

By default, it uses `.gitleaks.toml` and `.gitleaksignore` from the repository root, falling back to `composer_root`; otherwise it uses Gitleaks' built-in rules. `config`, `ignore_file`, and `baseline` accept explicit project-relative files. `discover_config: false` disables automatic config/ignore discovery. `rules` restricts enabled rules by their native IDs; custom rules and allowlists belong in TOML. Gitleaks resolves `[extend].path` relative to the repository root; list these transitive config files in `extra_inputs` so their changes invalidate a running check. Baselines are native JSON reports and suppress matches using Gitleaks' own matching semantics.

Gitleaks shares `scope.include` and `scope.exclude` with the other checkers. For example, a root `.env` is outside discovered Drupal custom roots; use `scope.include: ["."]` to include it. Checker `include`/`exclude` can further narrow the selected files. Archives and encoded payloads are not recursively scanned. No project files or baselines are generated or changed by the checker.

### Twig CS Fixer

Twig CS Fixer checks `.twig` templates. It uses `.twig-cs-fixer.php` or `.twig-cs-fixer.dist.php` from the project root. Without one it uses the default `TwigCsFixer` standard without `IncludeFunctionRule` and `CompactHashRule`, because Drupal templates use the `include` tag and long-form hashes. Other useful settings are `config`, `discover_config`, `extra_extensions`, and `extra_inputs` for files the project config loads.

Set `tools.twig-cs-fixer.autoload: auto` when a project config uses Drupal or other project token parsers, or set a relative autoloader path explicitly. This executes the trusted project Composer autoloader; it is disabled by default. The bundled Twig loader retains priority and its tool versions are recorded before project autoload. Changes to project autoload/config inputs invalidate the request context.

Twig CS Fixer checks style, not whether a template compiles. A template it cannot read at all is reported as `File.Invalid` even outside the changed lines.

### Composer validate

`composer-validate` checks the root `composer.json`, including whether `composer.lock` is up to date, when either file changed. In `--mode full` it always checks them. It also checks `composer.json` files of selected modules. Errors and an outdated lock are errors; other Composer warnings, such as exact version constraints, are warnings. Set `strict: true` to treat warnings as errors.

A root `composer.json` that is not valid JSON stops the check with exit code `2` before any checker runs, because PHPWarden reads the project layout from it.

### Composer audit

`composer-audit` checks the whole `composer.lock` on every run, whatever changed, because new advisories appear without code changes. Advisories come from the repositories in `composer.json`, such as Packagist and packages.drupal.org. Each advisory is a warning, so it does not fail the check; set `fail_on: warning` to block on them, for example in CI. Abandoned packages are reported as info; set `abandoned: ignore` to hide them.

Composer needs the network to fetch advisories. PHPWarden runs it in a separate container that sees only copies of `composer.json` and `composer.lock`; the other checkers stay offline. Results are cached in `~/.cache/phpwarden/advisories`:

- A cached result younger than `max_age_hours` (24) is used without the network.
- If fetching fails, a cached result younger than `max_stale_hours` (168) is used, and the report says so.
- Otherwise the check is unavailable, never clean. `offline: true` uses only the cache.

`environment.advisories` in the JSON report shows whether advisories came from the network or the cache, and when they were fetched. The check is optional by default, so working offline gives `PARTIAL` instead of a failure; set `required: true` in CI.

To accept a known advisory, list it with a reason and an optional end date:

```yaml
tools:
  composer-audit:
    ignore:
      - id: SA-CORE-2026-016
        reason: Patched in patches/core-xss.patch
        until: "2026-12-31"
```

`id` matches the advisory ID or its CVE. After `until`, the advisory is reported again.

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

`--format json` prints one report that follows [schemas/report-v1.schema.json](schemas/report-v1.schema.json). Its content and the exit codes do not depend on the text format.

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
