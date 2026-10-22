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

The project is in early development. The production CLI, Docker images, checkers, and agent integrations are not available yet.

The first milestone is to define the core contracts and establish automated tests before implementing the checks.
