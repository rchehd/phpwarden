---
type: map
title: CI workflow triggers and jobs
description: >-
  Checks validates branches and PRs; Build runs manually or on v* tags with CLI,
  images or all targets after the same commit passes checks.
tags:
  - ci
  - docker
  - release
kk_schema_version: 3
kk_id: map-ci-workflow-triggers-and-jobs
kk_derived_from:
  - README.md
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:map:0'
kk_relates_to:
  - practice-run-docker-integration-suites
kk_depends_on: []
kk_confidence: high
---
`.github/workflows/ci.yml` is **Checks**, running Go/native and fixture checks on every branch push and pull request, manual dispatch, or reusable workflow call. The Go job checks format and vet, runs race tests, builds the standalone binary, checks helper syntax and runs worker acceptance; the Python job runs Ruff and the fixture tests.

`.github/workflows/build.yml` is **Build**, running only through a manual **Actions → Build → Run workflow** dispatch or a pushed `v*` tag. It first invokes the reusable Checks workflow for the same commit. Manual `target` choices are `all` (default), `cli`, and `images`; `images` or `all` builds and tests all PHP 8.1–8.4 images. `publish_images` is allowed only on `main` with `images` or `all` and publishes the tested `static-php8.X-dev` images to GHCR. Version tags build both components and publish version-specific tags such as `static-php8.4-v0.1.0-rc.1` without moving the floating dev tags.

Manual `cli`/`all` and version-tag runs upload a `phpwarden-linux-amd64-<commit>` artifact with `phpwarden-linux-amd64.tar.gz` and `SHA256SUMS`. Job contents are also described in `docs/TESTING.md`, whose trigger description is outdated.

CLI presentation changes can use a CLI-only build. Worker, checker or tool compatibility changes require matching CLI and images; a complete distribution is built and tested together.

<!-- kk:related:start -->
# Related

- Related: [practice-run-docker-integration-suites](practice-run-docker-integration-suites.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
[2] 01a1693d-cb80-7003-ae47-1468fd6435b0:map:0
<!-- kk:citations:end -->
