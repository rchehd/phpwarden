---
type: map
title: PHPWarden CLI commands and exit-code contract
description: >-
  Static checks, setup, diagnostics, updates, project tests and performance
  commands; scope rules and stable exit codes.
tags:
  - cli
  - exit-codes
  - ci
kk_schema_version: 3
kk_id: map-cli-surface-and-exit-codes
kk_derived_from:
  - README.md
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:3'
kk_relates_to:
  - map-report-v1-json-contract
  - map-text-output-rule-groups-layout
  - practice-exit-codes-and-completeness-semantics
  - practice-scope-computation-and-reporting-rules
kk_depends_on: []
kk_confidence: high
---
The CLI exposes `init`, `check`, `fix` (alias for check --fix), `scope`, `config validate/show`, `resolve`, `prepare`, `doctor`, `updates`, `update`, `test`, `mutation`, `worker check`, `container`, `hook pre-commit`, `hook stop`, `instructions remove` and `performance` with setup/status/teardown, connect/disconnect and mcp subcommands. `--build-info` returns executable identity; `--licenses` returns bundled notices.

`check` is diff-scoped by default; --commits N selects the last N commits plus local edits without needing --mode diff. Explicit file/directory arguments cannot be combined with --mode, --base, --commits or --whole-files. JSON/compact retain all findings; --all expands text groups. Init creates config without overwriting existing files, offers checker/skill/instruction choices in terminals, and supports -n. Skills, project instructions, completion hooks and Git pre-commit remain independently selectable.

Exit codes: `0` no blocking findings, `1` blocking findings, `2` invalid configuration, arguments or project metadata (including an invalid root `composer.json`), `3` a required checker could not run or the project changed during the check, `130` interrupted. JSON output follows `schemas/report-v1.schema.json` and its content and exit codes do not depend on the text format.

Text output is `PARTIAL` (`complete: false` in JSON) when an optional checker could not run. `--all` and `--color` affect display only, never analysis, totals, caching or exit codes.

`updates` reports Composer, Drupal and PHP release/security metadata without updating packages; `update` manages the standalone executable through published releases. `resolve` and `doctor` do not pull images; `prepare` explicitly downloads the selected published worker without running checks. Test, mutation, update and performance commands have their own structured result families; the check-report schema is not their universal envelope.

<!-- kk:related:start -->
# Related

- Related: [map-report-v1-json-contract](map-report-v1-json-contract.md)
- Related: [map-text-output-rule-groups-layout](map-text-output-rule-groups-layout.md)
- Related: [practice-exit-codes-and-completeness-semantics](practice-exit-codes-and-completeness-semantics.md)
- Related: [practice-scope-computation-and-reporting-rules](../git-scope/practice-scope-computation-and-reporting-rules.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
[2] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:3
<!-- kk:citations:end -->
