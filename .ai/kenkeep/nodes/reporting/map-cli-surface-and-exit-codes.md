---
type: map
title: PHPWarden CLI commands and exit-code contract
description: >-
  Commands init, check, fix, scope, config validate/show, hook pre-commit; exit
  codes 0/1/2/3/130 are stable for CI.
tags:
  - cli
  - exit-codes
  - ci
kk_schema_version: 3
kk_id: map-cli-surface-and-exit-codes
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
The CLI exposes `init` (writes `.phpwarden.yaml`, optional `--pre-commit`), `check` (diff-scoped by default; `--base`, `--commits`, `--whole-files`, `--mode full`, `--only`, `--format text|compact|json`, `--all`, `--no-cache`, `--fix`), `fix` (alias for `check --fix`), `scope` (shows files and lines a check would cover), `config validate`/`config show`, and `hook pre-commit`.

Exit codes: `0` no blocking findings, `1` blocking findings, `2` invalid configuration, arguments or project metadata (including an invalid root `composer.json`), `3` a required checker could not run or the project changed during the check, `130` interrupted. JSON output follows `schemas/report-v1.schema.json` and its content and exit codes do not depend on the text format.

Text output is `PARTIAL` (`complete: false` in JSON) when an optional checker could not run. `--all` and `--color` affect display only, never analysis, totals, caching or exit codes.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
<!-- kk:citations:end -->
