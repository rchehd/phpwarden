---
type: map
title: Report v1 JSON structure and evolution rules
description: >-
  Shape of the check report v1: top-level keys, finding and diagnostic fields,
  ordering and compatibility rules.
tags:
  - report
  - json
  - schema
kk_schema_version: 3
kk_id: map-report-v1-json-contract
kk_derived_from:
  - docs/CORE-CONTRACTS.md
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:4'
kk_relates_to:
  - map-text-output-rule-groups-layout
  - practice-separate-failure-policy-from-findings
  - practice-exit-codes-and-completeness-semantics
kk_depends_on: []
kk_confidence: high
---
Top-level keys: `version: 1`, `run_id`, `verdict` (`pass|fail|error`), `complete`, `exit_code`, `profile`, `scope`, `environment`, `checks`, `findings`, `diagnostics`, plus optional `fixed_files` listing files actually changed by an explicit fix. `profile`, `scope` and `environment` may be null when an input error occurs before discovery. `checks` carry ID, status, required, fail_on and reason or null.

A finding has `checker`, `rule`, `kind` (`code|syntax|global`), `severity` (`error|warning|info`), `message`, `path`, `line`, `column`. Global findings have null path/line/column; code and syntax findings need a non-empty project-relative path, a positive line and a positive-or-null column. Line 0 is never invented. Diagnostics have a stable `code`, `stage`, `message` and checker ID or null, so execution problems never masquerade as code findings.

Findings sort by path, line, column, checker, rule, message; checks sort by ID; duplicate normalized findings are removed. Absolute temp or container paths and raw tool stdout never appear in finding paths. In JSON mode stdout holds exactly one JSON document, with logs on stderr. The config command envelope (`config validate/show`) is a separate response, not a check report. JSON evolution only adds optional fields; removing or changing semantics requires a new major protocol/schema version.

Findings can carry an optional SHA256 `fingerprint` and structured `trace` for security flows. These retain native locations rather than replacing execution diagnostics. The check report does not certify an analyzed Git commit or execution time: Markdown reports need companion evidence for that binding, otherwise freshness is unverified. `phpwarden-report` writes Markdown from JSON and optional existing auxiliary reports, keeping native totals/verdict separate from agent checklist observations; formatting alone does not authorize more analyses.

<!-- kk:related:start -->
# Related

- Related: [map-text-output-rule-groups-layout](map-text-output-rule-groups-layout.md)
- Related: [practice-separate-failure-policy-from-findings](practice-separate-failure-policy-from-findings.md)
- Related: [practice-exit-codes-and-completeness-semantics](practice-exit-codes-and-completeness-semantics.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
[2] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:4
<!-- kk:citations:end -->
