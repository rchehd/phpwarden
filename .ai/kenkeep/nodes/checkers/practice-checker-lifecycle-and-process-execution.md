---
type: practice
title: Run checkers through the shared lifecycle with strict process handling
description: >-
  Checkers register a descriptor and run argv-only with validated native exit
  codes and report structure.
tags:
  - checker
  - process
  - adapters
kk_schema_version: 3
kk_id: practice-checker-lifecycle-and-process-execution
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
A checker descriptor declares stable ID, schema/defaults, file types, environment requirements, `supports_fix`, `analysis_scope` (`selected|project`), `report_kinds` and adapter version. New checkers are registered with the checker registry and the profile; plugin and hook integrations stay unchanged.

Lifecycle: validate options, determine applicability, check requirements, prepare native config, execute, parse and normalize, apply shared reporting scope, evaluate fail policy. Processes run as an argv array with no shell interpolation, and the result keeps exit code, stdout, stderr and timeout/cancel state.

Adapters whitelist the native exit codes they know and validate the report structure; arbitrary JSON or the absence of an error word never proves success. Dependencies are checked per applicable checker (PHPStan needs project autoload, debug and whitespace do not), not by a global `vendor` check. There is no automatic `composer install`, no project DB/services and no host PHP/DDEV fallback.

Statuses are `passed`, `failed`, `skipped` (reason required: disabled, not_selected, no_files), `unavailable` (reason required) and `error`. Warnings keep their severity, and `fail_on: never` never hides an execution error. `required: true` does not force a checker to run without applicable files or outside an explicit `--only` selection.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
