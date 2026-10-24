---
type: map
title: Scenario fixture harness layout
description: >-
  JSON-described Git scenarios under tests/fixtures/scenarios, built into real
  temp Git repos by a Python harness.
tags:
  - testing
  - fixtures
  - git
kk_schema_version: 3
kk_id: map-scenario-fixture-harness
kk_derived_from:
  - docs/TESTING.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Scenarios live in `tests/fixtures/scenarios/<id>/` with `before/` (initial project), `change.patch` or `untracked/` (the change) and `scenario.json` (ID, Git state, changed files, expected findings, expected result for `diff` and `full`). The harness is `tests/support/scenarios.py` plus `tests/test_scenarios.py`.

The harness copies `before/` into a temp dir, creates a real Git repo with a baseline commit, then applies the patch or adds untracked files. The `state` field selects unstaged, staged, or committed-on-a-separate-branch. The temp dir is deleted after the test, and global Git configs and inherited `GIT_*` variables are ignored.

Untracked cases use `untracked/` because `git apply` would turn the added file into a tracked change. Names forbidden on Windows are stored under a portable name and mapped by the optional `untracked_renames` manifest field. The rename happens only inside the temp repo, so those scenarios need a filesystem that allows such names.

The `regressions` field links scenarios to `INV-*` inventory issues. Other checkers have their own fixture groups (`tests/fixtures/syntax`, `yaml`, `whitespace`, `drupal-config`, `phpmd`, `gitleaks` and others) that reuse the same Git harness.

<!-- kk:citations:start -->
# Citations

[1] [docs/TESTING.md](../../../docs/TESTING.md)
<!-- kk:citations:end -->
