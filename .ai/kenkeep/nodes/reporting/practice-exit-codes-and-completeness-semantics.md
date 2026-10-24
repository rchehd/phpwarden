---
type: practice
title: Map results to exit codes by fixed priority and track completeness
description: >-
  Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but
  set complete=false.
tags:
  - report
  - exit-codes
  - policy
kk_schema_version: 3
kk_id: practice-exit-codes-and-completeness-semantics
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Exit codes: 0 pass; 1 blocking findings reached `fail_on`; 2 usage/config/profile/base/selection/compatibility input error; 3 runner/internal/stale result or required checker unavailable/error (including global timeout); 130 interrupted (report best effort). Priority is interruption, input error, execution error, blocking finding, pass. A required error plus a finding gives exit 3 with both reasons kept in the report.

Optional (`required: false`) unavailable or error results are always recorded, set `complete: false` and show PARTIAL with a reason in text, but do not alone block exit 0; blocking findings already collected still give exit 1. A required failure always blocks. `complete` means all selected, enabled, applicable checkers finished with an up-to-date result; correct skips do not lower it and it does not claim disabled or unselected tools ran.

A worker crash must never yield a clean result through a missing report. The CLI ends a run once, and agent hooks translate the same exit code and report without their own scope or rule logic. `--only` validates IDs before execution and enables the chosen checkers even if disabled by default; unknown IDs or incompatible flags exit 2, so a typo can never produce a clean result.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
