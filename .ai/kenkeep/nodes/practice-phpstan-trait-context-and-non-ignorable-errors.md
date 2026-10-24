---
type: practice
title: PHPStan trait context suffix and non-ignorable errors
description: >-
  Normalize only a validated trait context suffix; ignorable:false is a code
  finding, not an analyzer failure.
tags:
  - phpstan
  - adapter
  - diagnostics
kk_schema_version: 3
kk_id: practice-phpstan-trait-context-and-non-ignorable-errors
kk_derived_from:
  - docs/BUGFIXES-2026-10-24.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
PHPStan appends `(in context of class ...)` to native keys for trait diagnostics. The adapter normalizes only a validated context suffix, enforces the selected physical paths, merges contextual diagnostics and keeps native total validation.

An error with `ignorable: false` (for example `return.missing`) is a code diagnostic and must be reported as a finding. Only internal/parse identifiers and missing positions mark the analysis incomplete.

<!-- kk:citations:start -->
# Citations

[1] [docs/BUGFIXES-2026-10-24.md](../../../docs/BUGFIXES-2026-10-24.md)
<!-- kk:citations:end -->
