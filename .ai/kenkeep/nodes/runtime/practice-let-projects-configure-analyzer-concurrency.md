---
type: practice
title: Let projects configure analyzer concurrency
description: >-
  Projects choose sequential, bounded or unrestricted worker concurrency
  according to available resources.
tags:
  - runner
  - config
  - concurrency
kk_schema_version: 3
kk_id: practice-let-projects-configure-analyzer-concurrency
kk_derived_from:
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:practice:1'
kk_relates_to:
  - practice-project-config-owns-rules
  - practice-checker-lifecycle-and-process-execution
kk_depends_on: []
kk_confidence: high
---
Analyzer concurrency belongs in project configuration so powerful servers can run all worker analyzers in parallel. `runner.concurrency` uses `1` for sequential execution, a positive integer for a concurrency limit, and `0` for all worker analyzers concurrently. The default remains `1`. Reports retain stable ordering regardless of completion order.

<!-- kk:related:start -->
# Related

- Related: [practice-project-config-owns-rules](../configuration/practice-project-config-owns-rules.md)
- Related: [practice-checker-lifecycle-and-process-execution](../checkers/practice-checker-lifecycle-and-process-execution.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 01a1693d-cb80-7003-ae47-1468fd6435b0:practice:1
<!-- kk:citations:end -->
