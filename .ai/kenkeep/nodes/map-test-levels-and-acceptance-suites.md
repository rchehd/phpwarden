---
type: map
title: Test levels and acceptance suites
description: >-
  Four test levels from repo quality to real tools in images, plus the worker,
  Docker and launcher acceptance modules.
tags:
  - testing
  - docker
  - acceptance
kk_schema_version: 3
kk_id: map-test-levels-and-acceptance-suites
kk_derived_from:
  - docs/TESTING.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Levels: (1) repo quality, i.e. Ruff plus `unittest` over the Python harness; (2) Go unit tests of the core, where tool processes may be replaced by controlled doubles (crash, timeout, missing executable, invalid JSON); (3) CLI acceptance, which runs the real PHPWarden on a prepared Git project with an explicit base ref and compares exit code and JSON report; (4) integration of real tools and images, mandatory before publishing an image. Mocks do not replace level 4.

Acceptance modules: `tests.acceptance.worker_cli` builds the binary and runs the real worker plus helpers natively. `tests.acceptance.docker_worker` runs the same scenarios in a real local image, selected with `PHPWARDEN_TEST_IMAGE`. `tests.acceptance.launcher_cli` extracts the compiled binary from the dev image and runs the public `check`. Each checker's fixtures run in 4 scope modes (diff, full, whole-files, paths) through all three suites.

Acceptance assertions compare checker, rule, relative path, line, verdict, `complete` and exit code. They never depend on ANSI output, absolute temp paths, timing or third-party message text. Fake-executable tests are marked separately from real-tool runs, and a missing Go or PHP fails explicitly instead of skipping.

<!-- kk:citations:start -->
# Citations

[1] [docs/TESTING.md](../../../docs/TESTING.md)
<!-- kk:citations:end -->
