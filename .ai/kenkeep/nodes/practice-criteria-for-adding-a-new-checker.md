---
type: practice
title: Criteria a new third-party tool must meet before becoming a checker
description: >-
  New checkers need their own settings shape, license review, a defined diff
  scope and own verdict/completeness; opt-in advisory first.
tags:
  - checkers
  - integration
  - licensing
kk_schema_version: 3
kk_id: practice-criteria-for-adding-a-new-checker
kk_derived_from:
  - docs/TOOL-CANDIDATES.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Every new checker has its own parameter shape; only an extension of an existing tool (for example PHPStan strict rules) may be a sub-form of that tool instead of a separate run. Tools that add coverage on top of existing ones are justified by unique findings, not by overlap with PHPStan, PHPCS or PHPMD.

Before packaging, the distribution obligations of the pinned version's license must be checked and notices/sources shipped alongside (LGPL for Semgrep, AGPL-3.0-or-later for Exakat CE). A separate CLI does not remove that obligation.

The adapter, not the tool, decides the verdict and completeness: a tool exiting 0 or emitting a clean report is never proof of a complete clean run, because some tools skip unreadable files or only log rule errors. Tools with no file/line selection need a scope-filtering step and a defined diff-mode behaviour. New tools start opt-in and advisory, and need negative fixtures for Drupal false positives before any blocking policy.

<!-- kk:citations:start -->
# Citations

[1] [docs/TOOL-CANDIDATES.md](../../../docs/TOOL-CANDIDATES.md)
<!-- kk:citations:end -->
