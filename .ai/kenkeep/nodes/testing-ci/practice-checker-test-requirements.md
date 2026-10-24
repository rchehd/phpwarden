---
type: practice
title: Cover every new checker with fixtures and failure tests
description: >-
  Each checker needs clean and failing fixtures, setting-driven cases, failure
  injection, and real-tool integration.
tags:
  - testing
  - checkers
  - fixtures
kk_schema_version: 3
kk_id: practice-checker-test-requirements
kk_derived_from:
  - docs/TESTING.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
For every new checker add a clean example and a failing example, settings that change the outcome (disabled/only, fail_on, include/exclude), and launch-failure cases. Cover the scope modes (diff, full, whole-files, paths) through worker, Docker and launcher suites. New adapters need their own fixtures and real tool integrations, not only doubles.

Add scenarios proving that a required checker with missing dependencies or an unusable report never yields success. For fixes, assert the resulting file contents, that a rerun makes no further changes, and that nothing outside the allowed paths changed. For parallel runs, assert temp/cache isolation and that results are fresh after a code change.

Write minimal fixtures by hand and keep them in Git so each failure is explained by one scenario. A generator is acceptable later only for version matrices and large combination counts.

When a checker adds a toolset, update the expected capabilities, toolset and preset in the manifest tests. Older toolsets and protocols are rejected by metadata.

<!-- kk:citations:start -->
# Citations

[1] [docs/TESTING.md](../../../../docs/TESTING.md)
<!-- kk:citations:end -->
