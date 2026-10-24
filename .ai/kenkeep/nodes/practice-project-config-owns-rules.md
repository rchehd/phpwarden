---
type: practice
title: 'Store profile and rules in the project''s .phpwarden.yaml, not account settings'
description: >-
  Profile, PHP target, rules and blocking policy live in the project-root
  config; native client settings are only adapters.
tags:
  - config
  - project
  - profile
kk_schema_version: 3
kk_id: practice-project-config-owns-rules
kk_derived_from:
  - docs/CONCEPT.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
`.phpwarden.yaml` sits at the analyzed project's root (version: 1, committable to Git) and is the single source of rules for plugins, standalone CLI and CI. A plugin may be installed once per user, but profile, PHP target, level, rulesets, exclusions and blocking policy belong to each repository, so one Claude/Codex account never couples rules across repos.

Native client fields (e.g. Claude `userConfig`) and settings forms are adapters or editors of this file, not a hidden account-wide store, and the core must not depend on Claude-specific storage. Existing native configs (phpstan.neon, phpcs.xml, ...) keep their roles; generated adapter configs and runtime state live in a temp/cache dir and never become a second rules source. Only personal UI and Docker-access preferences may be machine-level.

Init must not overwrite an existing config or native tool configs, and an invalid existing config returns a diagnostic instead of being replaced by defaults. Profile is never carried over from another project, and unsupported or missing profiles require setup or a clear message rather than applying Drupal checks to other projects. Config creation is atomic and safe under concurrent agent starts, does not git-add the file, and in CI a plain check never silently creates config.

Show the effective value of each setting with its origin: profile default, project config, native tool config or explicit run override. Also documented in `docs/TOOLS-SETTINGS.md`.

<!-- kk:citations:start -->
# Citations

[1] [docs/CONCEPT.md](../../../docs/CONCEPT.md)
<!-- kk:citations:end -->
