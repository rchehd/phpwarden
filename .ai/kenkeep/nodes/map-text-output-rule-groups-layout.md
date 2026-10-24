---
type: map
title: 'Default text output: rule-grouped findings layout'
description: >-
  Text report leads with verdict and totals, then groups findings by
  severity+checker+rule, capped per group unless --all.
tags:
  - cli
  - output
  - text-format
kk_schema_version: 3
kk_id: map-text-output-rule-groups-layout
kk_derived_from:
  - docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Default text output opens with the verdict, full finding totals, exit meaning and analysis completeness. Skipped checks, setup failures and coverage notes are printed before any findings. Group identity is severity + checker + rule; errors precede warnings/info, and groups within a severity sort by checker/rule. Counts always describe the complete group.

Within a group each file appears once; identical messages share a compact position list, while differing messages keep their details beside their positions. Each group shows at most 10 findings by default, and every truncated group ends with an explicit omitted finding/file count. `--all` (check, fix, worker check) expands text groups and is rejected with JSON/compact, which already carry every finding.

Messages and position lists wrap at 100 characters on word boundaries; unbroken paths and identifiers stay intact, so width is not a hard guarantee. Original messages and full IDs remain in JSON; compact output is a separate, complete format with its own summary.

<!-- kk:citations:start -->
# Citations

[1] [docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md](../../../docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md)
<!-- kk:citations:end -->
