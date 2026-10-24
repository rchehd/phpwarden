---
type: practice
title: Exakat CE is deliberately not integrated into the image or default pipeline
description: >-
  Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and
  unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
tags:
  - exakat
  - checkers
  - scope
kk_schema_version: 3
kk_id: practice-exakat-ce-deliberately-not-integrated
kk_derived_from:
  - docs/TOOL-CANDIDATES.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Exakat CE is not added to the worker image or default pipeline. Reasons: it needs PHP extensions, a Gremlin server on Java, its own writable graph and results storage (a heavy complication for the worker image); its dependency graph requires whole-context analysis, so inputs cannot be trimmed to diff hunks; the CE tree has CompatibilityPHP83 but no PHP84 set, so the 8.1-8.4 matrix is unconfirmed; and its Drupal marker only flags namespace usage, not Drupal-aware dynamic entry points. Its AGPL-3.0-or-later license also needs a distribution review.

Its benefit beyond PHPStan, PHPCompatibility, PHPMD and dead-code is unmeasured. Reconsider only when a concrete set of unique rules is needed or a current CE release confirms the PHP matrix, then run a separate spike on licensing, Drupal false positives, time, RAM, graph/tmp storage and SARIF completeness.

<!-- kk:citations:start -->
# Citations

[1] [docs/TOOL-CANDIDATES.md](../../../../docs/TOOL-CANDIDATES.md)
<!-- kk:citations:end -->
