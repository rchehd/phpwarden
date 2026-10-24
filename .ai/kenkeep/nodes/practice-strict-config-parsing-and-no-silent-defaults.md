---
type: practice
title: Reject ambiguous config input instead of guessing
description: >-
  Config parsing is strict and never silently infers versions, ranges or base
  refs.
tags:
  - config
  - validation
kk_schema_version: 3
kk_id: practice-strict-config-parsing-and-no-silent-defaults
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Config YAML may only use JSON-compatible types. Duplicate keys, custom tags, aliases/merge keys, unknown fields, unknown checker IDs and an unknown `version` are rejected. Empty arrays never mean "inherit"; a present `include` must contain at least one path.

Never silently choose a value: no "latest" framework version, no inferred PHP range from ambiguous Composer constraints, no guessed `main`/`master` base. `framework.version: auto` requires a real exact version from the project; without one, `check` returns an input error and `init` asks. `check` never creates config automatically; `init` is a separate atomic operation that never overwrites existing config or native files.

The rationale is that a wrong guess yields a false clean result, which is worse than an explicit input error.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
