---
type: map
title: CI workflow triggers and jobs
description: >-
  Go and fixture jobs run on every push/PR; Docker builds, image publishing and
  CLI artifacts only on manual runs or v* tags.
tags:
  - ci
  - docker
  - release
kk_schema_version: 3
kk_id: map-ci-workflow-triggers-and-jobs
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
`.github/workflows/ci.yml` runs Go/native and fixture checks on every branch push and pull request. The Go job checks format and vet, runs race tests, builds the standalone binary, checks helper syntax and runs worker acceptance; the Python job runs Ruff and the fixture tests.

Docker builds run only through a manual **Actions → CI → Run workflow** dispatch or a pushed `v*` tag, after the base tests pass. A manual run builds and tests all PHP 8.1–8.4 images by default; `publish_images` (allowed only on `main`) publishes the tested `static-php8.X-dev` images to GHCR. Version tags publish version-specific tags such as `static-php8.4-v0.1.0-rc.1` without moving the floating dev tags.

Manual and version-tag runs also upload a `phpwarden-linux-amd64-<commit>` artifact with `phpwarden-linux-amd64.tar.gz` and `SHA256SUMS`. Job contents are also described in `docs/TESTING.md`, whose trigger description is outdated.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../README.md)
<!-- kk:citations:end -->
