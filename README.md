# PHPWarden

Static quality checks for PHP projects, with one command, one report and no PHP on the host.

PHPWarden runs more than twenty analyzers inside a Docker image. By default it checks only the lines you changed against a base branch, so you can adopt it on an existing project without first fixing all legacy code. If an analyzer cannot run, the result is reported as incomplete and never as clean.

Drupal 10 and 11 are currently the only supported frameworks. Symfony, Laravel and WordPress profiles are planned.

> **Status:** pre-release. No stable version has been published yet, and only Linux amd64 is supported.

## What it does

- **Checks your changes:** coding standards, types, security patterns, secrets, debug leftovers, duplication, dead code and architecture rules.
- **Fits local work and CI:** the CLI, Git hook, agent hooks and CI produce the same report and exit codes.
- **Stays isolated:** analyzers run in a pinned image, with no network and the project mounted read-only.
- **Helps coding agents:** skills for Claude Code, Codex and Cursor cover review, architecture rules, upgrades, test gaps and reports.
- **Goes beyond linting:** assess dependency updates, run project tests with coverage and mutation testing, and read SPX performance profiles.

## Requirements

- Linux amd64, Git and a local Docker daemon.
- A Drupal project on PHP 8.1 to 8.4, with `composer install` already run.

## Quick start

Download `install.sh`, `phpwarden-linux-amd64.tar.gz` and `SHA256SUMS` from a Build artifact, then:

```sh
sh install.sh --archive ./phpwarden-linux-amd64.tar.gz
cd /path/to/your/project
phpwarden init
phpwarden check
```

The installer verifies the archive and places the CLI in `~/.local/bin`. `init` writes `.phpwarden.yaml`, and the first `check` downloads the matching worker image. See [installation](guide/installation.md) for Compose, Composer and npm options.

## Common commands

| Command | Purpose |
|---|---|
| `phpwarden check` | Check changed lines against the base branch |
| `phpwarden check --commits 2` | Check the last two commits and uncommitted changes |
| `phpwarden check --mode full` | Check all custom code |
| `phpwarden check --format json` | Write the full report as JSON |
| `phpwarden fix` | Apply PHPCS and Twig CS Fixer corrections |
| `phpwarden doctor` | Explain why the setup is not ready |
| `phpwarden updates` | Assess available updates and security advisories |
| `phpwarden test --coverage` | Run project unit tests with coverage |
| `phpwarden mutation` | Measure how well tests catch code changes |
| `phpwarden init --pre-commit` | Install the Git pre-commit hook |
| `phpwarden init --agents all -n` | Install agent skills |

Exit codes: `0` passed, `1` blocking findings, `2` invalid input, `3` incomplete or failed execution, `130` interrupted.

## Configure a project

`phpwarden init` detects the PHP version and project layout and writes every available setting to `.phpwarden.yaml`. Detailed rules stay in each analyzer's own config, such as `phpstan.neon` or `phpcs.xml`. See [configuration](guide/configuration.md).

## Documentation

- [Installation and updates](guide/installation.md)
- [Configuration, Git hook and monorepos](guide/configuration.md)
- [Running checks and reading results](guide/checks.md)
- [Checkers](guide/checkers.md)
- [Agent skills, instructions and hooks](guide/agents.md)
- [Diagnostics and dependency updates](guide/updates.md)
- [Project tests and mutation testing](guide/testing.md)
- [Performance profiles (experimental)](guide/performance.md)
- [Development and distribution](guide/development.md)

## License

PHPWarden is released under the [MIT License](LICENSE). Bundled tools keep their own licenses. Run `phpwarden --licenses` for CLI third-party notices.
