---
type: map
title: 'Default text output: quality categories'
description: >-
  Normal text groups findings, execution failures and final check statuses by
  quality category; JSON and compact retain technical IDs.
tags:
  - cli
  - output
  - text-format
kk_schema_version: 3
kk_id: map-text-output-rule-groups-layout
kk_derived_from:
  - docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:map:2'
kk_relates_to:
  - practice-exit-codes-and-completeness-semantics
  - practice-separate-failure-policy-from-findings
kk_depends_on: []
kk_confidence: high
---
Default text output opens with the verdict, full finding totals, exit meaning and analysis completeness. Details and the final Checks checklist use quality categories instead of analyzer names: Security, Code standards, Correctness, Dependencies, Maintainability, Drupal quality, Architecture and Upgrades. Each registered checker belongs to one category; global diagnostics use Environment. Findings and execution failures share a consistent presentation inside each category. The tool-based Skipped list and tool/rule subtitle lines are omitted. Category statuses aggregate failures and incomplete coverage so a successful check cannot hide another required check that could not run. Within a category, finding group identity remains severity + checker + rule; errors precede warnings/info, and groups within a severity sort by checker/rule. Counts always describe the complete group.

Within a group each file appears once; identical messages share a compact position list, while differing messages keep their details beside their positions. Each group shows at most 10 findings by default, and every truncated group ends with an explicit omitted finding/file count. `--all` (check, fix, worker check) expands text groups and is rejected with JSON/compact, which already carry every finding.

Messages and position lists wrap at 100 characters on word boundaries; unbroken paths and identifiers stay intact, so width is not a hard guarantee. Original messages and full IDs remain in JSON; compact output is a separate, complete format with its own summary.

<!-- kk:related:start -->
# Related

- Related: [practice-exit-codes-and-completeness-semantics](practice-exit-codes-and-completeness-semantics.md)
- Related: [practice-separate-failure-policy-from-findings](practice-separate-failure-policy-from-findings.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md](../../../../docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md)
[2] 01a1693d-cb80-7003-ae47-1468fd6435b0:map:2
<!-- kk:citations:end -->
