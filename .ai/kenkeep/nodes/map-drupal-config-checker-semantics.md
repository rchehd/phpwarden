---
type: map
title: 'Drupal module config checker: top-level uuid/_core rules'
description: >-
  The drupal-config checker flags top-level uuid/_core in module config only,
  whole-file, natively in Go; site exports are excluded.
tags:
  - drupal-config
  - checker
  - yaml
kk_schema_version: 3
kk_id: map-drupal-config-checker-semantics
kk_derived_from:
  - docs/FIRST-MILESTONE.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
The drupal-config checker flags top-level `uuid` and `_core` keys in Drupal module config files. It is a whole-file structural check, so findings are not limited to changed hunks. Nested keys and text occurrences are allowed, while YAML merge keys and aliases do not bypass the check.

Files are classified as conventional, custom or standalone module config. Site config exports are never included automatically. The adapter is native Go and needs no project runtime. Malformed YAML makes the semantic run incomplete rather than clean.

<!-- kk:citations:start -->
# Citations

[1] [docs/FIRST-MILESTONE.md](../../../docs/FIRST-MILESTONE.md)
<!-- kk:citations:end -->
