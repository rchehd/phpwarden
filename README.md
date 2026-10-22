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

The project is in early development. The Go CLI can validate project configuration and show effective settings with their sources. Code checks, Docker images, and agent integrations are not available yet.

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

Go tests exercise configuration validation, defaults and overrides, project selection in monorepos and Git worktrees, and the actual compiled CLI's JSON output and exit codes. The JSON schema is embedded in the binary; configuration validation needs no network access or Docker.

Python tests validate the code-check scenario fixtures and their declared expectations. They do not run the planned code checkers yet. Each scenario creates a temporary Git repository from initial files and a patch or untracked files, then removes it after the test.

GitHub Actions checks Go formatting, vet, race tests, and a standalone build, alongside Ruff and the Python fixture tests, on pushes and pull requests.
