# PHPWarden

PHP quality checks for local development, CI, and coding agents.

PHPWarden is being designed as a standalone CLI with Docker-based tools and adapters for Claude Code and Codex. Drupal will be the first framework profile; additional PHP frameworks can follow through separate profiles.

## Planned capabilities

- Run quality tools from versioned Docker images without requiring host PHP or Composer.
- Check changed code by default, with full-project and explicit-path modes.
- Share project-specific settings through `.phpwarden.yaml` across CLI, CI, and agent integrations.
- Extend the tool through modular checkers, framework profiles, and runners.
- Distinguish code findings from unavailable tools and execution failures.

## Development status

The project is in early development. The Go CLI validates configuration, discovers Drupal layout and locked versions, inspects Git scope, selects a development toolset, and runs a PHP debug checker through Docker. A local development image contains PHP 8.4 and Git. Published images, automatic pulls, and agent integrations are not available yet.

## Build and try the CLI

Requires Go 1.26.5. From the repository root:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
bin/phpwarden config validate --config examples/drupal.phpwarden.yaml
bin/phpwarden config show --config examples/drupal.phpwarden.yaml --format json
```

For a project, save its configuration as `.phpwarden.yaml` in the project root. The CLI finds the nearest configuration from the current directory up to the Git worktree root, or accepts an explicit `--project` or `--config` path. Explicit paths also allow configuration validation outside Git. Relative command-line paths are resolved from the caller's directory.

Validation checks YAML structure, the [v1 schema](schemas/config-v1.schema.json), PHP target ranges, and path boundaries. It rejects unknown fields and checker IDs, duplicate keys, aliases, and numeric PHP versions. Currently only the `debug` checker is implemented.

`config show` includes discovered project facts, core and Drupal defaults, explicit project overrides, and their provenance. Its `pending` list identifies unresolved values. `config validate` checks configuration syntax/schema and path boundaries without inspecting Composer metadata. Discovery validates Composer PHP requirements and compares installed Drupal version metadata when available. Full dependency integrity and release image compatibility remain future work. The development launcher currently requires PHP runtime and target `"8.4"`.

Configuration commands return exit `0` for valid input, `2` for usage/configuration errors, and `3` for output failures. With `--format json`, stdout contains a single JSON document. These configuration responses are separate from the worker's code-check report.

## Inspect analysis scope

In a Git project with `.phpwarden.yaml` and discovered or explicit source roots:

```sh
phpwarden scope --base origin/main --format json
phpwarden scope --mode full
phpwarden scope --base origin/main --whole-files
phpwarden scope web/modules/custom/demo
```

The diff scope compares the final working tree with the merge base and includes committed, staged, unstaged, and nonignored untracked changes. Without an explicit base, it uses `scope.base`, then the locally available `origin/HEAD`; it never guesses a branch or fetches history. Full scope selects tracked and nonignored untracked files within the configured source roots. Explicit paths are relative to the caller's directory and still respect roots and exclusions.

JSON `scope.files` contains project-relative paths. `scope.line_map` maps each path to inclusive `{start, end}` ranges; `null` means all lines, while `[]` means a selected file has no added lines. New files and rename destinations select all lines. Deleted files are omitted. `--whole-files` expands reporting to all lines of changed files and does not select unchanged files.

Scope inspection uses discovered Drupal custom directories when `scope.include` is omitted. Explicit includes replace those defaults; core/contrib/vendor exclusions remain additive even for `include: [.]`. If no custom roots can be discovered, set explicit existing paths. Symlinks and nested submodules are not traversed. Non-UTF-8 filenames and unavailable Git history produce errors. This command inspects scope and does not run code checkers or declare the code clean.

## Drupal discovery

Discovery reads files and directory metadata without loading Drupal, running Composer scripts, installing dependencies, or accessing a database. It runs for `config show`, `scope`, `resolve`, and both check commands. Use `config show --format json` to inspect `effective.project`, resolved configuration, provenance, and pending fields.

The framework version comes from exact `drupal/core` or `drupal/core-recommended` entries in `composer.lock`, with installed core metadata as a fallback when the lock does not supply a version. A leading `v` is normalized. Conflicting metadata or a conflict with explicit `framework.version` are errors. Constraints, development branches, and prerelease versions are not guessed as stable versions. Without a usable discovered version, an explicit exact version is accepted with `project_config` provenance; otherwise the version stays pending. The current debug-only toolset can run with a pending version; future framework-dependent adapters will require verified compatibility.

The web root comes from Composer's `extra.drupal-scaffold.locations.web-root`, then `extra.installer-paths` for `type:drupal-core`. Otherwise a unique root, `web/`, or `docroot/` layout is recognized through `core/lib/Drupal.php` or custom directory conventions. Conflicting or ambiguous layouts return an error. These metadata fields follow [Drupal's Composer configuration](https://www.drupal.org/docs/develop/using-composer/manage-dependencies).

Default source roots are existing `modules/custom`, `themes/custom`, and `profiles/custom` directories, including `sites/*` variants. Literal custom installer directories ending in `/{$name}` are supported. Core, conventional contrib directories, and Composer's custom `vendor-dir` are excluded. Contrib installer paths outside a `contrib` directory exclude individual typed locked packages, preserving adjacent custom code. Installer templates containing other variables require a supported literal mapping; arbitrary Composer plugin layouts are not resolved yet.

Project paths remain relative to the configuration root. Discovery rejects path traversal, symlink traversal, duplicate JSON keys, and malformed relevant metadata. Composer file hashes and layout facts are compared before/after analysis, including missing metadata files and newly appearing custom/multisite directories; changes make the result stale.

## Composer compatibility

Discovery reads root `require.php`, `require-dev.php`, `conflict.php`, lock platform requirements, and locked package PHP requirements/conflicts. Production requirements constrain both tool runtime and the entire configured target minor range. Development requirements constrain runtime, allowing production code to target an older PHP than its development dependencies need. This check does not inspect extensions or solve the dependency graph.

The Go parser supports stable numeric constraints with comparisons, AND/OR, hyphen ranges, wildcards, caret, and tilde syntax. Unsupported branch/stability/alias expressions fail explicitly. Its stable-version behavior is checked against 660 precomputed results from [composer/semver 3.4.4](https://github.com/composer/semver/tree/3.4.4); running tests needs no Composer installation. See [Composer's constraint syntax](https://getcomposer.org/doc/articles/versions.md).

Each configured target minor must have at least one patch satisfying all production constraints. If only some patches qualify, `php.target.patch` stays pending. A runtime minor with patch restrictions similarly adds `php.runtime.patch` during discovery. `resolve` checks the selected image's exact PHP version; the worker reports `environment.php_version` and checks the observed version again. Runtime patch uncertainty is removed after that check; remaining compatibility pending fields appear in JSON and text reports. These fields do not mean all target patches have been tested.

`config.platform.php` is recorded as Composer's simulated dependency-resolution version, checked against declared requirements, and kept separate from PHPWarden's runtime. It never replaces `php.runtime`. Lock `platform-overrides`, extension/platform packages, and lock content-hash validation are not evaluated yet. [Composer documents the platform setting here](https://getcomposer.org/doc/06-config.md#platform).

When available, `<vendor-dir>/composer/installed.json` is read as Composer 1's package array or Composer 2's package object. `effective.project.installed` reports `absent`, `not_installed`, `unverified`, `observed`, or `verified`. Verification compares Drupal version metadata with the lock; it does not verify installed source files or all package versions. A no-dev installation may omit core when core appears only in the lock's dev packages. Missing vendor remains valid for the debug checker. Existing conflicting, incomplete required-core, malformed, or unsafe installed metadata produces an input error.

Project autoload and executable `installed.php` are never loaded. If only PHP metadata exists, its presence is reported as unverified and `framework.installed_version` stays pending. Installed JSON content/presence joins freshness checks, so changes during analysis invalidate the result even though vendor is excluded from code scope.

## Debug worker

The worker command runs directly in its execution environment and requires PHP with the tokenizer extension. Its PHP minor version must match `php.runtime`. It can run inside a preselected image or directly during development. Public `phpwarden check` uses Docker and has no host PHP fallback.

In a configured project with discovered or explicit source roots:

```sh
phpwarden worker check --only debug --base origin/main --format json
phpwarden worker check --mode full
phpwarden worker check --whole-files
phpwarden worker check web/modules/custom/demo
```

The checker tokenizes PHP source without executing it or loading project dependencies. Defaults prohibit `var_dump`, `print_r`, `dpm`, `dsm`, and `kint`; `tools.debug.functions` replaces that list. It recognizes direct calls, including multiline calls and leading backslashes, and ignores comments, string literals, declarations, object/static methods, attributes, and first-class callable acquisition. Variable calls, imported function aliases, and full namespace/shadowing resolution are not covered. It does not check PHP syntax or JavaScript debug statements.

`tools.debug.enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude` control execution. `--only debug` explicitly enables the checker even when disabled in configuration. Reporting uses the shared Git line map. No project `vendor` directory is required.

Reports follow the [v1 report schema](schemas/report-v1.schema.json), which is embedded and validated before output. Exit codes are `0` for no blocking findings, `1` for blocking findings, `2` for input errors, and `3` for required checker/process failures or changed inputs. Optional failures return `complete: false` and a visible `PARTIAL` text result. The worker checks file/configuration checksums and recomputes scope after analysis. Each helper run owns and removes its temporary directory.

Reports include `environment.framework_version` when an exact version is available. Direct `worker check` reports have no resolved image/toolset metadata. Public `check` reports identify the inspected immutable local image ID and development toolset. Release compatibility validation remains pending.

## Check through Docker

After building the Go CLI and development image, run these commands in your configured Git project:

```sh
phpwarden resolve --format json
phpwarden check --base origin/main --only debug --format json
phpwarden check --mode full
phpwarden check --whole-files
phpwarden check web/modules/custom/demo
```

`resolve` selects the bundled development toolset offline. It currently accepts only Linux amd64, profile `drupal`, and PHP runtime/target `"8.4"`. This toolset runs only the tokenizer-based debug checker and does not claim compatibility with a particular installed Drupal version. PHPStan and other framework-dependent tools will require a tested release compatibility catalog.

`check` computes Git scope on the host, verifies the local image's platform/protocol/toolset/PHP labels, and starts it by its inspected image ID. It sends a versioned request with effective settings, selected files/lines, input checksums, and the expected PHP version over stdin. The worker verifies inputs and its actual PHP version; the launcher also rechecks host checksums and Git scope. Linked worktrees and monorepos work without mounting external Git metadata into the worker.

The launcher requires host Git and a local Docker Unix socket, with no host PHP, Composer, or Go requirement for the built binary. It mounts the project read-only, uses your UID/GID, disables container networking, and gives each run its own temporary filesystem and container name. Interruptions return `130`; global timeouts and runner failures return `3`, with best-effort removal of the run's container. An empty scope or disabled checker needs no Docker process after configuration/toolset validation.

The development image must already exist under `phpwarden:debug-php8.4-dev`; a missing image returns an error with build instructions. There is no registry fallback or pull for this unpublished tag. Remote Docker contexts and other platforms are currently rejected. New launcher/worker protocol changes require rebuilding the development image.

## Run the development Docker image

Build from this repository with Docker; Go and PHP are installed inside the build/runtime images:

```sh
docker build -f docker/Dockerfile -t phpwarden:debug-php8.4-dev .
```

From the Git project you want to check, with an existing `.phpwarden.yaml`, run:

```sh
docker run --rm --pull=never --network=none --read-only \
  --cap-drop=ALL --security-opt=no-new-privileges:true \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m,mode=1777 \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$PWD,target=/workspace,readonly" \
  phpwarden:debug-php8.4-dev worker check --only debug --format json
```

Set PHP runtime/target to `"8.4"` and configure an available diff base. Source roots can be discovered or supplied through `scope.include`. Append `--mode full` for a full scan or `--base origin/main` to override the diff base. The command returns the worker's exit code and JSON report.

This image contains only the debug checker, PHP 8.4.26, and Git. It requires no host PHP, Composer, or Go, and no project `vendor` directory. The image defaults to UID/GID 10001; the command above uses your UID/GID to read private project directories. Check mode mounts the project read-only and uses a private temporary filesystem. Git trusts only the known `/workspace` mount through the image's system configuration.

Use a local Docker daemon with access to the source directory. Mount the entire Git repository for monorepos and pass `--project /workspace/path/to/project`. Linked worktrees or separate Git directories whose metadata lives outside the mount need additional path mapping; the launcher will handle those separately. Container paths in findings remain relative to the configured project.

The tag is local and unpublished. Base images are pinned by digest, while Alpine packages still come from their package repositories. Release package locks, platform/framework compatibility entries, published digests, and automatic pulls remain future work. The current image is tested on Linux amd64 and does not establish a Drupal version support matrix.

## Repository checks

Requires Go 1.26.5, PHP 8.4 with tokenizer, Python 3.12, and Git. From the repository root:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements-dev.txt
.venv/bin/python -m ruff check tests
.venv/bin/python -m ruff format --check tests
.venv/bin/python -m unittest discover -s tests -v
go vet ./...
go test -race ./...
php -l internal/checker/debug/helper.php
.venv/bin/python -m unittest tests.acceptance.worker_cli -v
```

Go tests exercise configuration validation, defaults and overrides, project selection in monorepos and Git worktrees, and the actual compiled CLI's JSON output and exit codes. Scope tests also materialize the existing patch fixtures and verify file/line selection, unusual filenames, deletion/rename behavior, exclusions, and merge-base semantics. The JSON schema is embedded in the binary; configuration validation needs no network access or Docker.

Python fixture tests validate the scenario data and Git states. The separate worker acceptance suite builds the real CLI, runs all six fixtures in four scope modes with the real PHP helper, and compares semantic findings and exit codes. It also injects process failures, checks optional policies, and tests configuration overrides.

The Docker suite runs the same six fixtures in four modes against the real image, checks read-only mounts and the default non-root user, verifies operation without host PHP/Go, and covers concurrent containers, runtime mismatch, and monorepo paths. Run it explicitly after building the image; it requires Python, Git, and Docker, without host PHP or Go:

```sh
PHPWARDEN_TEST_IMAGE=phpwarden:debug-php8.4-dev \
  python3 -m unittest tests.acceptance.docker_worker -v
python3 -m unittest tests.acceptance.launcher_cli -v
```

The launcher suite extracts the actual CLI binary from the built image and runs public `check` across the same 24 fixture combinations with no PHP or Go on the client PATH. It also checks linked worktrees, monorepos, offline resolve, and settings overrides. Go tests inject missing Docker/images, invalid reports, changed inputs, cancellation, and timeout failures.

Drupal discovery acceptance covers root/web/docroot/custom web roots, multisite custom directories, automatic source roots, explicit broad roots with profile exclusions, locked version reporting, malformed metadata, and version conflicts. These are small synthetic Git projects; they do not establish compatibility with a full installed Drupal application.

Composer compatibility tests cover production/dev distinctions, target gaps, intersecting patch restrictions, simulated platform settings, optional vendor, Composer 1/2 installed metadata, custom vendor paths, no-dev layouts, metadata changes, and actual worker PHP patch verification. Docker acceptance exercises matching/mismatching installed Drupal versions and image patch incompatibility without executing project autoload.

GitHub Actions checks Go formatting, vet, race tests, a standalone build, PHP helper syntax, and worker acceptance alongside Ruff and Python fixture validation. A separate Docker job builds the Linux amd64 development image and runs both Docker and public launcher suites without installing host Go/PHP. The worker test environment uses PHP 8.4. Image publishing and a version/platform matrix are not configured yet.
