---
type: practice
title: Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold
description: >-
  Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector
  2.6.2+, so both stay pinned.
tags:
  - pins
  - deptrac
  - rector
  - docker
kk_schema_version: 3
kk_id: practice-tool-version-pins-and-rationale
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Deptrac is pinned to 4.4.0 (with PHPStan 2.3.1) because it is the newest release compatible with the worker's full PHP 8.1-8.4 matrix; Deptrac 4.5 and newer require PHP 8.2.

Drupal Rector 1.1.3 explicitly conflicts with Rector 2.6.2 and newer, so the isolated bundle locks Rector 2.6.1 (with PHPStan 2.3.1). ComposerRequireChecker is 4.7.1 on PHP 8.1 and 4.16.1 on PHP 8.2-8.4.

Isolated bundles (Deptrac, Rector) keep separate locks so they cannot disturb project dependencies.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../README.md)
<!-- kk:citations:end -->
