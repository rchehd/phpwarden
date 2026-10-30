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
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:0'
kk_relates_to:
  - practice-let-projects-configure-analyzer-concurrency
  - practice-project-config-owns-rules
  - practice-config-root-paths-and-precedence
  - map-cli-surface-and-exit-codes
kk_depends_on: []
kk_confidence: high
---
`.phpwarden.yaml` lives in the project root. Required: `version: 1`, `profile`, `php.runtime` (`major.minor` string selecting the image), `php.target.min/max` (`major.minor` strings, inclusive, min <= max). Optional: `framework.version` (`auto` or exact `major.minor.patch`), `scope.{mode,base,include,exclude,whole_files}`, `runner.{timeout_seconds,concurrency,image_pull,result_cache}`, `tools.<id>`, `agents.completion`, `updates`, `tests` and `mutation`. Runner defaults are 300 seconds, concurrency 1 (0 permits all applicable checks concurrently), image_pull missing and result_cache false. PHP versions are always strings.

Every `tools.<id>` shares `enabled`, `required` (default true), `fail_on` (`error|warning|never`, default error; warning means errors and warnings), `timeout_seconds` (default 120, capped by the remaining global timeout) and `include`/`exclude` (narrowing only, never widening the global scope). Checker-specific options sit at the same level as the shared ones, not in a nested options block. Checkers with native configs also accept `config` (explicit project-relative path) and `discover_config`.

The PHP target is separate from the runtime image and does not mean the syntax checker runs on every target version. Executable JSON Schema is `schemas/config-v1.schema.json`; the loader adds semantic range and path checks.

`tests` owns PHPUnit executable/config discovery, literal test paths or a native suite, timeouts and optional coverage include/exclude, line-coverage and method-CRAP thresholds. `mutation` owns Infection config, overall timeout, threads, per-mutant timeout, mutators and MSI thresholds. These are separate commands, not static-check tools. `updates` controls metadata timeout, freshness/staleness and offline mode. `agents.completion` controls an explicitly installed completion hook; enabled defaults to true, max_attempts to 2 and timeout_seconds to 300. No hook is installed merely because this block exists.

<!-- kk:related:start -->
# Related

- Related: [practice-let-projects-configure-analyzer-concurrency](../runtime/practice-let-projects-configure-analyzer-concurrency.md)
- Related: [practice-project-config-owns-rules](practice-project-config-owns-rules.md)
- Related: [practice-config-root-paths-and-precedence](practice-config-root-paths-and-precedence.md)
- Related: [map-cli-surface-and-exit-codes](../reporting/map-cli-surface-and-exit-codes.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
[2] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:0
<!-- kk:citations:end -->
