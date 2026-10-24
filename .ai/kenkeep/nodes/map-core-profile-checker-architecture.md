---
type: map
title: 'Modular architecture: core, profiles, checkers, runners, integrations'
description: >-
  Core components, the profile and checker layers, and the rule that core stays
  framework-agnostic.
tags:
  - architecture
  - modules
  - profiles
  - checkers
kk_schema_version: 3
kk_id: map-core-profile-checker-architecture
kk_derived_from:
  - docs/CONCEPT.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Components: Config, Discovery, Scope, Orchestration, Checker, Runner, Reporting, Integration (per-AI-client hooks and skills) and Profile (compatible tool versions plus initial rules). Empty modules for future candidates are not created up front.

The core and universal PHP checkers carry no mandatory Drupal assumptions. The Drupal profile adds layout discovery, file extensions, custom-code directories, core/contrib exclusions, Drupal Coder, phpstan-drupal, module config checks and Rector sets. Base PHP defaults may be reused by the profile.

Each checker declares its supported files, required environment capabilities, config, execution rules and result format. Adding a checker must not require changing agent integrations; Claude and Codex packages share the core and checker modules rather than duplicating them.

<!-- kk:citations:start -->
# Citations

[1] [docs/CONCEPT.md](../../../docs/CONCEPT.md)
<!-- kk:citations:end -->
