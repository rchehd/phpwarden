---
type: practice
title: Keep analyzer progress focused on the user operation
description: >-
  Interactive progress names the current operation and elapsed time without
  exposing Docker as a stage detail.
tags:
  - cli
  - progress
  - usability
kk_schema_version: 3
kk_id: practice-keep-analyzer-progress-focused-on-the-user-operation
kk_derived_from:
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:practice:2'
kk_relates_to:
  - map-launcher-worker-execution-model
  - map-text-output-rule-groups-layout
kk_depends_on: []
kk_confidence: high
---
Interactive check progress shows the operation and elapsed time, for example `Running analyzers · 10 files · 0:03`. Omit `in Docker` from the user-facing stage label because it is an internal implementation detail. This wording rule concerns progress labels; useful technical details in errors and configuration paths remain available.

<!-- kk:related:start -->
# Related

- Related: [map-launcher-worker-execution-model](../architecture/map-launcher-worker-execution-model.md)
- Related: [map-text-output-rule-groups-layout](map-text-output-rule-groups-layout.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 01a1693d-cb80-7003-ae47-1468fd6435b0:practice:2
<!-- kk:citations:end -->
