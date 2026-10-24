---
type: practice
title: Build all PHP images before running Docker acceptance suites
description: >-
  Run docker buildx bake, then docker_worker per runtime and launcher_cli;
  rebuild the image together with the CLI.
tags:
  - testing
  - docker
  - ci
kk_schema_version: 3
kk_id: practice-run-docker-integration-suites
kk_derived_from:
  - docs/TESTING.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
From the repo root run `docker buildx bake`, then `for v in 8.1 8.2 8.3 8.4; do PHPWARDEN_TEST_IMAGE=phpwarden:static-php$v-dev python3 -m unittest tests.acceptance.docker_worker -v; done`, then `python3 -m unittest tests.acceptance.launcher_cli -v` (all with the `rtk proxy` prefix). The launcher suite uses the bundled dev tag.

Rebuild the images together with the CLI whenever a checker, capability or toolset changes. The suites reject images whose toolset or capabilities metadata is stale, so an old build fails rather than silently passing.

The Docker suites need only Docker, Python and Git, not host Go or PHP, and fail explicitly when the image or daemon is missing. Each test container has its own name and is removed even after a timeout. Direct container mounts need the whole Git repository, while the public `check` passes host scope and does not.

<!-- kk:citations:start -->
# Citations

[1] [docs/TESTING.md](../../../../docs/TESTING.md)
<!-- kk:citations:end -->
