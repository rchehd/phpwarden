---
type: map
title: CI workflow triggers and jobs
description: >-
  Path-filtered Checks, PR/manual Sandbox and Mutation, manual/tag Build targets
  and tag-only release publication.
tags:
  - ci
  - docker
  - release
kk_schema_version: 3
kk_id: map-ci-workflow-triggers-and-jobs
kk_derived_from:
  - README.md
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:map:0'
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:6'
kk_relates_to:
  - practice-run-docker-integration-suites
  - practice-run-python-and-go-test-suites
  - map-test-levels-and-acceptance-suites
  - map-scenario-fixture-harness
kk_depends_on: []
kk_confidence: high
---
`.github/workflows/ci.yml` is **Checks**, triggered by branch pushes, pull requests, manual dispatch and reusable calls. `.github/actions/changes` and `tools/ci/changes.py` select relevant Go/native and Python jobs from changed paths; manual/build validation forces the complete suite. Documentation and knowledge-only changes skip expensive jobs. The Go job runs formatting/vet/race, license inventory, source build, worker acceptance and Composer/npm delivery tests; the Python job runs Ruff and fixture/infrastructure tests. A final Checks complete job enforces selected results.

`.github/workflows/build.yml` is **Build**, running only through a manual **Actions → Build → Run workflow** dispatch or a pushed `v*` tag. It first invokes the reusable Checks workflow for the same commit. Manual `target` choices are `all` (default), `cli`, `images`, and `plugins`; `images` or `all` builds and tests all PHP 8.1–8.4 images. `publish_images` is allowed only on `main` with `images` or `all` and publishes the tested `static-php8.X-dev` images to GHCR. Version tags build both components and publish version-specific tags such as `static-php8.4-v0.1.0-rc.1` without moving the floating dev tags.

Manual cli/all and version-tag runs upload the standalone archive, checksums and Composer/npm/Compose integrations as Actions artifacts. After image publication the CLI embeds the exact matching digest-pinned worker manifest; a CLI-only source build cannot auto-pull workers on a clean machine. plugins/all and tags upload three native plugin ZIPs plus a self-contained catalog ZIP and checksums. The tag-only Release job waits for Docker, CLI and plugin jobs and publishes verified assets; a manual build never creates a GitHub Release.

CLI presentation changes can use a CLI-only build. Worker, checker or tool compatibility changes require matching CLI and images; a complete distribution is built and tested together.

`sandbox.yml` runs relevant pull-request/manual Drupal 10/11 installation scenarios across Compose, DDEV, bare Docker and host PHP, plus independent Docker-less installer/profile-reader scenarios. `mutation.yml` runs relevant pull-request/manual Drupal 10/11 x PHP 8.3/8.4 mutation scenarios after reusable Checks. Neither starts on an ordinary main push. Build all also exercises installed sandboxes before publishing workers. These runners and artifact tests do not establish live native agent-client acceptance.

<!-- kk:related:start -->
# Related

- Related: [practice-run-docker-integration-suites](practice-run-docker-integration-suites.md)
- Related: [practice-run-python-and-go-test-suites](practice-run-python-and-go-test-suites.md)
- Related: [map-test-levels-and-acceptance-suites](map-test-levels-and-acceptance-suites.md)
- Related: [map-scenario-fixture-harness](map-scenario-fixture-harness.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
[2] 01a1693d-cb80-7003-ae47-1468fd6435b0:map:0
[3] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:6
<!-- kk:citations:end -->
