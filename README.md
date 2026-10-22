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

The project is in early development. The Go CLI can validate configuration, show effective settings with their sources, and inspect Git analysis scope. Code checks, Docker images, and agent integrations are not available yet.

## Build and try the CLI

Requires Go 1.26.5. From the repository root:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
bin/phpwarden config validate --config examples/drupal.phpwarden.yaml
bin/phpwarden config show --config examples/drupal.phpwarden.yaml --format json
```

For a project, save its configuration as `.phpwarden.yaml` in the project root. The CLI finds the nearest configuration from the current directory up to the Git worktree root, or accepts an explicit `--project` or `--config` path. Explicit paths also allow configuration validation outside Git. Relative command-line paths are resolved from the caller's directory.

Validation checks YAML structure, the [v1 schema](schemas/config-v1.schema.json), PHP target ranges, and path boundaries. It rejects unknown fields and checker IDs, duplicate keys, aliases, and numeric PHP versions. Currently only the `debug` configuration section is recognized; the checker itself is not implemented yet.

`config show` includes core and Drupal defaults, explicit project overrides, and their provenance. Its `pending` list identifies values that need framework discovery. Configuration validation does not verify installed Drupal dependencies, Composer constraints, or available Docker images yet. The example PHP versions illustrate the configuration format and do not declare a supported image combination.

Configuration commands return exit `0` for valid input, `2` for usage/configuration errors, and `3` for output failures. With `--format json`, stdout contains a single JSON document. These configuration responses are separate from the future code-check report.

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

## Repository checks

Requires Go 1.26.5, Python 3.12, and Git. From the repository root:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements-dev.txt
.venv/bin/python -m ruff check tests
.venv/bin/python -m ruff format --check tests
.venv/bin/python -m unittest discover -s tests -v
go vet ./...
go test -race ./...
```

Go tests exercise configuration validation, defaults and overrides, project selection in monorepos and Git worktrees, and the actual compiled CLI's JSON output and exit codes. Scope tests also materialize the existing patch fixtures and verify file/line selection, unusual filenames, deletion/rename behavior, exclusions, and merge-base semantics. The JSON schema is embedded in the binary; configuration validation needs no network access or Docker.

Python tests validate the code-check scenario fixtures and their declared expectations. They do not run the planned code checkers yet. Each scenario creates a temporary Git repository from initial files and a patch or untracked files, then removes it after the test.

GitHub Actions checks Go formatting, vet, race tests, and a standalone build, alongside Ruff and the Python fixture tests, on pushes and pull requests.
