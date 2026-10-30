---
type: map
title: Shared development toolset compatibility metadata
description: >-
  docker-bake.json supplies the shared development toolset revision to the CLI,
  manifest entries and Docker image labels.
tags:
  - toolset
  - docker
  - build
kk_schema_version: 3
kk_id: map-shared-development-toolset-compatibility-metadata
kk_derived_from:
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:map:0'
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:2'
kk_relates_to:
  - practice-image-resolution-and-run-isolation
  - practice-select-image-before-container-start
  - practice-run-docker-integration-suites
  - map-profile-version-image-model
  - map-checker-inventory
  - map-launcher-worker-execution-model
kk_depends_on: []
kk_confidence: high
---
`docker-bake.json` (`variable.PHPWARDEN_TOOLSET.default`) is the single source of the development toolset compatibility revision. `buildinfo.go` embeds it; `internal/toolset.DevelopmentID` combines it with the PHP runtime, and bundled development manifest entries derive their IDs from that value. Published release manifest entries retain explicit toolset IDs.

`docker-bake.hcl` defines the PHP image variants and passes the matching `TOOLSET_ID` build argument to `docker/Dockerfile`, which records it in `io.phpwarden.toolset`. The launcher compares that label with the selected toolset and refuses a mismatch.

The revision identifies CLI/worker/toolset compatibility independently of PHP, framework and PHPWarden release versions. When compatibility changes, update the shared value and rebuild both CLI and images. A Bake configuration check without building can verify that each runtime receives the same derived ID expected by the CLI.

Image protocol and checker capabilities are validated separately from the toolset label. Enabling a checker in YAML cannot add a missing image capability: current development workers do not include the experimental Psalm bundle. Runtime labels, embedded release manifests and capability declarations must agree; a configuration-only opt-in cannot bypass that contract.

<!-- kk:related:start -->
# Related

- Related: [practice-image-resolution-and-run-isolation](practice-image-resolution-and-run-isolation.md)
- Related: [practice-select-image-before-container-start](practice-select-image-before-container-start.md)
- Related: [practice-run-docker-integration-suites](../testing-ci/practice-run-docker-integration-suites.md)
- Related: [map-profile-version-image-model](map-profile-version-image-model.md)
- Related: [map-checker-inventory](../checkers/map-checker-inventory.md)
- Related: [map-launcher-worker-execution-model](../architecture/map-launcher-worker-execution-model.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 01a1693d-cb80-7003-ae47-1468fd6435b0:map:0
[2] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:2
<!-- kk:citations:end -->
