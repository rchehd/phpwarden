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

The project is in early development. The Go CLI can validate configuration, inspect Git scope, and run a PHP debug checker through the explicit worker command. A development Docker image runs that worker with PHP 8.4 and Git. The Docker launcher, published images, and agent integrations are not available yet.

## Build and try the CLI

Requires Go 1.26.5. From the repository root:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
bin/phpwarden config validate --config examples/drupal.phpwarden.yaml
bin/phpwarden config show --config examples/drupal.phpwarden.yaml --format json
```

For a project, save its configuration as `.phpwarden.yaml` in the project root. The CLI finds the nearest configuration from the current directory up to the Git worktree root, or accepts an explicit `--project` or `--config` path. Explicit paths also allow configuration validation outside Git. Relative command-line paths are resolved from the caller's directory.

Validation checks YAML structure, the [v1 schema](schemas/config-v1.schema.json), PHP target ranges, and path boundaries. It rejects unknown fields and checker IDs, duplicate keys, aliases, and numeric PHP versions. Currently only the `debug` checker is implemented.

`config show` includes core and Drupal defaults, explicit project overrides, and their provenance. Its `pending` list identifies values that need framework discovery. Configuration validation does not verify installed Drupal dependencies, Composer constraints, or available Docker images yet. The example PHP versions illustrate the configuration format and do not declare a supported image combination.

Configuration commands return exit `0` for valid input, `2` for usage/configuration errors, and `3` for output failures. With `--format json`, stdout contains a single JSON document. These configuration responses are separate from the worker's code-check report.

## Inspect analysis scope

In a Git project with `.phpwarden.yaml` and explicit `scope.include` paths:

```sh
phpwarden scope --base origin/main --format json
phpwarden scope --mode full
phpwarden scope --base origin/main --whole-files
phpwarden scope web/modules/custom/demo
```

The diff scope compares the final working tree with the merge base and includes committed, staged, unstaged, and nonignored untracked changes. Without an explicit base, it uses `scope.base`, then the locally available `origin/HEAD`; it never guesses a branch or fetches history. Full scope selects tracked and nonignored untracked files within the configured source roots. Explicit paths are relative to the caller's directory and still respect roots and exclusions.

JSON `scope.files` contains project-relative paths. `scope.line_map` maps each path to inclusive `{start, end}` ranges; `null` means all lines, while `[]` means a selected file has no added lines. New files and rename destinations select all lines. Deleted files are omitted. `--whole-files` expands reporting to all lines of changed files and does not select unchanged files.

Scope inspection currently requires explicit, existing `scope.include` roots; automatic Drupal layout discovery and core/contrib exclusions are still pending. Adjust the example's `web/modules/custom` path to your project. Core vendor/node_modules exclusions remain active. Symlinks and nested submodules are not traversed. Non-UTF-8 filenames and unavailable Git history produce errors. This command inspects scope and does not run code checkers or declare the code clean.

## Debug worker

The worker command runs directly in its execution environment and requires PHP with the tokenizer extension. Its PHP minor version must match `php.runtime`. The intended deployment is inside a preselected tool image; direct execution is currently available for development and testing. The public `phpwarden check` Docker launcher is still pending, and does not fall back to host PHP.

In a configured project with explicit source roots:

```sh
phpwarden worker check --only debug --base origin/main --format json
phpwarden worker check --mode full
phpwarden worker check --whole-files
phpwarden worker check web/modules/custom/demo
```

The checker tokenizes PHP source without executing it or loading project dependencies. Defaults prohibit `var_dump`, `print_r`, `dpm`, `dsm`, and `kint`; `tools.debug.functions` replaces that list. It recognizes direct calls, including multiline calls and leading backslashes, and ignores comments, string literals, declarations, object/static methods, attributes, and first-class callable acquisition. Variable calls, imported function aliases, and full namespace/shadowing resolution are not covered. It does not check PHP syntax or JavaScript debug statements.

`tools.debug.enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude` control execution. `--only debug` explicitly enables the checker even when disabled in configuration. Reporting uses the shared Git line map. No project `vendor` directory is required.

Reports follow the [v1 report schema](schemas/report-v1.schema.json), which is embedded and validated before output. Exit codes are `0` for no blocking findings, `1` for blocking findings, `2` for input errors, and `3` for required checker/process failures or changed inputs. Optional failures return `complete: false` and a visible `PARTIAL` text result. The worker checks file/configuration checksums and recomputes scope after analysis. Each helper run owns and removes its temporary directory.

Current worker reports have no resolved image/toolset metadata. Framework discovery and image compatibility validation are still pending; this is a working debug worker rather than a complete Docker release.

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

Set `php.runtime` to `"8.4"` and configure explicit `scope.include` roots and an available diff base. The example configuration uses PHP 8.3; adjust it before using this image. Append `--mode full` for a full scan or `--base origin/main` to override the diff base. The command returns the worker's exit code and JSON report.

This image contains only the debug checker, PHP 8.4.26, and Git. It requires no host PHP, Composer, or Go, and no project `vendor` directory. The image defaults to UID/GID 10001; the command above uses your UID/GID to read private project directories. Check mode mounts the project read-only and uses a private temporary filesystem. Git trusts only the known `/workspace` mount through the image's system configuration.

Use a local Docker daemon with access to the source directory. Mount the entire Git repository for monorepos and pass `--project /workspace/path/to/project`. Linked worktrees or separate Git directories whose metadata lives outside the mount need additional path mapping; the launcher will handle those separately. Container paths in findings remain relative to the configured project.

The tag is local and unpublished. Base images are pinned by digest, while Alpine packages still come from their package repositories. Release package locks, platform/framework compatibility entries, resolved image metadata, and automatic pull/run remain future work. The current image is tested on Linux amd64 and does not establish a Drupal version support matrix.

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
```

GitHub Actions checks Go formatting, vet, race tests, a standalone build, PHP helper syntax, and worker acceptance alongside Ruff and Python fixture validation. A separate Docker job builds the Linux amd64 development image and runs the Docker suite. The worker test environment uses PHP 8.4. Image publishing and a version/platform matrix are not configured yet.
