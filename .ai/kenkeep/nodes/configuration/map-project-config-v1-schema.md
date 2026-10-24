---
type: map
title: .phpwarden.yaml v1 fields and checker settings
description: >-
  Fields of the strict v1 project config, including the shared per-checker
  settings block.
tags:
  - config
  - schema
kk_schema_version: 3
kk_id: map-project-config-v1-schema
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
`.phpwarden.yaml` lives in the project root. Required: `version: 1`, `profile`, `php.runtime` (`major.minor` string selecting the image), `php.target.min/max` (`major.minor` strings, inclusive, min <= max). Optional: `framework.version` (`auto` or exact `major.minor.patch`), `scope.{mode,base,include,exclude,whole_files}`, `runner.timeout_seconds` (default 300), `tools.<id>`. PHP versions are always strings.

Every `tools.<id>` shares `enabled`, `required` (default true), `fail_on` (`error|warning|never`, default error; warning means errors and warnings), `timeout_seconds` (default 120, capped by the remaining global timeout) and `include`/`exclude` (narrowing only, never widening the global scope). Checker-specific options sit at the same level as the shared ones, not in a nested options block. Checkers with native configs also accept `config` (explicit project-relative path) and `discover_config`.

The PHP target is separate from the runtime image and does not mean the syntax checker runs on every target version. Executable JSON Schema is `schemas/config-v1.schema.json`; the loader adds semantic range and path checks.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
