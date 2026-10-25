---
name: phpwarden-doctor
description: Diagnose PHPWarden setup failures involving project dependencies, native configs, Docker and CLI/image compatibility. Use when checks cannot run or the environment is incomplete.
---

# PHPWarden doctor

Run the project's configured executable with `doctor --format json`. Inspect `.phpwarden.yaml` and the repository's runtime instructions; use `config show --format json` when discovery works.

Explain each failed prerequisite and its concrete correction. Distinguish missing project dependencies from missing tool bundles, invalid native configs from unsupported syntax, Docker access from image compatibility, and application PHP from the worker runtime. Honor Composer-root paths in monorepos and the project's environment wrapper when suggesting dependency commands.

If the report cannot validate a particular native config without executing its analyzer, say so and use a targeted check when authorized. Do not guess that all configs are valid from file existence alone. Keep errors visible; do not disable checks, weaken policies or install unrelated dependencies to hide the failure.

Make reversible corrections within the requested task. Installing dependencies, rebuilding images, changing runtime, modifying credentials and publishing require authorization within that task. Do not change global Docker/configuration or erase user files merely to resolve an environmental failure. Report what was verified, what remains blocked and the next concrete command.
