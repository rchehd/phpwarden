---
type: map
title: 'Profile, framework version, PHP runtime/target, toolset and image model'
description: >-
  One profile per framework; versions, runtime and tool sets are resolved via a
  compatibility table to a digest-pinned image.
tags:
  - profile
  - image
  - versions
  - docker
kk_schema_version: 3
kk_id: map-profile-version-image-model
kk_derived_from:
  - docs/CONCEPT.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
One framework has one user-facing profile (`drupal` now). Drupal 10/11 are not separate profiles; version-specific defaults and compatibility entries are internal to the profile. Distinct concepts: profile, framework version, PHP runtime (inside the container), PHP target (code compatibility), toolset (pinned binaries, extensions, rule packages) and image (runtime plus toolset, pinned by digest). Image tags such as `drupal11-php8.3` name image variants, not profile IDs.

Image selection: read profile and explicit settings; take the real framework version from Composer lock/installed packages (composer.json constraints are not exact); determine PHP runtime and target; pick a tested toolset from the plugin release manifest; pull by digest; show effective versions and image in forms and reports. If no compatible entry exists or settings conflict with the project, fail with a clear error and never substitute the nearest tag or latest.

Changing PHPStan level, exclusions, already-available rules, baseline or project ruleset is config-only and needs no image rebuild. A different PHP runtime, incompatible tool versions or a missing rule package needs a supported image variant. Tool dependencies are installed at image build time, never on a settings change, and the toolset version is versioned separately from Drupal/PHP. A single image may serve several entries if verified compatible.

<!-- kk:citations:start -->
# Citations

[1] [docs/CONCEPT.md](../../../docs/CONCEPT.md)
<!-- kk:citations:end -->
