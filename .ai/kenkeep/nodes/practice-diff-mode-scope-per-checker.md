---
type: practice
title: Decide per checker whether diff mode reports the whole file or changed lines
description: >-
  Syntax-like checkers report any line of a selected file; style and cross-file
  checkers filter to changed lines.
tags:
  - diff
  - scope
  - checkers
kk_schema_version: 3
kk_id: practice-diff-mode-scope-per-checker
kk_derived_from:
  - docs/TOOLS-SETTINGS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
In `diff` mode the default report scope is changed lines, but each checker must define its own scope explicitly. PHP syntax, YAML syntax and `drupal-config` analyze the whole selected file and report errors on any line, including deletion-only diffs. The `whitespace` checker reports only added lines in `diff` and all lines in `full`, paths or whole-files runs.

Some checkers analyze a wider context than the diff and filter the report afterward: PHPCS and Semgrep scan the whole file and filter findings to selected lines, Deptrac analyzes the extended graph and filters by the dependency's source line, and EventHorizon config analysis reads all top-level `.yml` files in the config directory.

When the cause and the reported location can differ (a changed taint source with an unchanged sink, a changed architecture rule affecting unchanged callers), `diff` cannot see the problem and `whole-files` or `full` is needed. Legacy-wide metrics such as the total duplicate percentage must not block a diff run.

<!-- kk:citations:start -->
# Citations

[1] [docs/TOOLS-SETTINGS.md](../../../docs/TOOLS-SETTINGS.md)
<!-- kk:citations:end -->
