---
type: map
title: Go single-module layout and profile contract
description: >-
  One Go module holds launcher, worker and shared packages; profiles own
  framework knowledge, core stays framework-agnostic.
tags:
  - architecture
  - profile
  - go
kk_schema_version: 3
kk_id: map-component-boundaries-and-profile-contract
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Launcher and worker are one Go binary with public launcher commands and separate worker commands. Shared packages live under `internal/`: `config`, `discovery`, `profile`, `scope`, `checker`, `runner`, `report`. Schemas are exported to `schemas/` and embedded in the binary. Users need only Git and a Docker CLI with a daemon, never Go, PHP or Composer. The Docker runner supports only a local Docker context with bind mounts. PHP helpers may exist inside the image but never become host dependencies.

A profile supplies identity and preset version, discovery facts with their sources, extra file classification, defaults (source roots, excludes, checker defaults), compatibility references and validation. Only `drupal` is registered; `php` and `laravel` return an unsupported-profile error, never a Drupal fallback.

Drupal specifics (file extensions, discovery, debug functions, module config rules, `internal/profile/drupal`) belong to the profile. Git, JSON reporting, Docker and process execution must not know the Drupal layout. When changing this, verify that no framework-specific logic leaked into scope, report or runner.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
