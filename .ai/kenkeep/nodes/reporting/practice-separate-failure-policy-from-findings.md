---
type: practice
title: 'Keep fail_on and required separate: findings policy vs. execution failure'
description: >-
  fail_on governs which findings block; a required checker that cannot run still
  fails and is never silently clean.
tags:
  - checkers
  - fail-on
  - severity
kk_schema_version: 3
kk_id: practice-separate-failure-policy-from-findings
kk_derived_from:
  - docs/TOOLS-SETTINGS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
`fail_on` decides which findings block completion (errors, also warnings, or none). It never hides an execution failure of a required tool. `required` means that inability to run an applicable check makes the run incomplete and unsuccessful.

No applicable files is a legitimate skip, not a `required` violation (for example PHPStan with no PHP files). A missing autoload or an incompatible config is a different situation and is an execution failure.

Do not rewrite native warnings into errors. Preserve the native severity and apply blocking policy separately. Timeouts, crashes, malformed output and missing runtimes are execution failures; an optional checker failing leaves the run `complete=false` rather than clean.

<!-- kk:citations:start -->
# Citations

[1] [docs/TOOLS-SETTINGS.md](../../../../docs/TOOLS-SETTINGS.md)
<!-- kk:citations:end -->
