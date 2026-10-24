---
type: practice
title: Deptrac adapter aborts the report when any file fails to parse
description: >-
  Deptrac continues after parser errors; the adapter must abort on syntax-error
  events and verify the parsed AST graph.
tags:
  - deptrac
  - checker
  - adapter
kk_schema_version: 3
kk_id: practice-deptrac-adapter-aborts-on-parser-failures
kk_derived_from:
  - docs/VALIDATION-2026-10-23.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Deptrac reports parser failures as events and may keep analysing, which yields a partial dependency graph that looks like a clean result. The adapter therefore aborts the report on the AST file syntax error event instead of emitting findings from the remaining files.

The adapter also reconciles the result against the AST graph that was actually parsed, so a run that silently skipped files is never reported as passing.

<!-- kk:citations:start -->
# Citations

[1] [docs/VALIDATION-2026-10-23.md](../../../docs/VALIDATION-2026-10-23.md)
<!-- kk:citations:end -->
