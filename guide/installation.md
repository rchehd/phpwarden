# Installation

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

Builds also produce two Linux amd64 project packages. Both contain the exact same CLI as the downloadable archive and verify its SHA256 before every launch. They install no analyzer dependencies and run no install scripts. With a published compatibility manifest, the first `check` downloads the selected digest-pinned worker. Source/development packages require matching preloaded development workers and never download mutable tags. Package-managed CLI updates use Composer/npm rather than `phpwarden update`.

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

Manual snapshots use a Composer version such as `dev-0.0.0-dev.ga38dff3`; copy the exact value from the archive's `composer.json` into both the repository and requirement. Stable, alpha, beta and RC versions retain their normal version strings. Other prerelease labels receive the `dev-` prefix because Composer accepts fewer prerelease formats than npm. The embedded CLI keeps its original version and worker manifest.

```sh
composer require --dev rchehd/phpwarden:0.1.0-rc.1 --no-scripts
vendor/bin/phpwarden init -n --pre-commit
vendor/bin/phpwarden check --commits 2
```

Commit `composer.json` and `composer.lock`. `composer install --no-scripts` reinstalls the pin; change the package repository version/URL and requirement together for an upgrade or rollback. Keep the project's trusted installer plugins enabled: disabling them can relocate existing Drupal packages. PHPWarden adds no Composer plugins or scripts. Downloaded assets should first be verified against the matching `SHA256SUMS`; the wrapper independently checks its embedded executable. Existing hooks are preserved; new hooks call the installed package wrapper, which remains at a stable project path across package updates.

Package wrappers support host installations with a local Docker daemon. Installing them inside an application container alone does not supply Docker access or map its paths to the host. Use the separate Compose integration for that workflow.

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
