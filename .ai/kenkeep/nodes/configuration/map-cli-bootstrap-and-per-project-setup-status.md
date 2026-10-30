---
type: map
title: CLI bootstrap and per-project setup status
description: >-
  Setup skill discovers managed or user CLI, installs verified artifacts on
  request and separates config, Docker and worker readiness.
tags:
  - config
  - agents
  - cli
kk_schema_version: 3
kk_id: map-cli-bootstrap-and-per-project-setup-status
kk_derived_from:
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:8'
kk_relates_to:
  - map-project-config-v1-schema
  - map-config-and-image-selection
  - practice-strict-config-parsing-and-no-silent-defaults
  - practice-project-config-owns-rules
  - map-cli-surface-and-exit-codes
kk_depends_on: []
kk_confidence: high
---
`internal/agentskills/skills/phpwarden-setup` supplies a host shell bootstrap and the root install.sh as a derived, byte-checked asset. On Linux amd64 the helper uses GNU shell utilities without Go/PHP/Composer/npm/Python. CLI discovery prefers an explicit path, project vendor/bin, project node_modules/.bin, PATH, then ~/.local/bin; an unusable selected candidate fails rather than silently falling back. It preserves managed packages and existing executables.

Status only probes bounded build identity and prerequisites, then the skill can use config show, resolve and doctor. Config presence, selected image, Docker-command presence, daemon readiness and verified local image availability are separate observations. Explicit setup authorizes the relevant user-local CLI install and project init; status, ordinary review or plugin update alone does not. Existing .phpwarden.yaml and native configs remain authoritative; init uses --agents none --instructions none --hooks none when the plugin already supplies skills.

Install supports an available release, offline archive or exact successful official Build run. The CI path requires authenticated gh, matching executable commit and a published-worker manifest; plugin-only and CLI-only source artifacts do not satisfy clean-machine auto-pull. Release downloads require curl. The canonical installer validates checksums, archive identity, platform and destination, without sudo; requested updates use the supported executable/package manager flow rather than implicit replacement.

Projects normally share one host CLI and image cache while selecting their own profile/runtime/worker. A published CLI can pull its selected missing digest; source builds require preloaded compatible development workers. Setup/status does not install host prerequisites, build images, add hooks/MCP or start analyzers. prepare is an explicit download action and overrides image_pull never for that operation. Formal plugin-update compatibility ranges and native client acceptance remain unverified.

<!-- kk:related:start -->
# Related

- Related: [map-project-config-v1-schema](map-project-config-v1-schema.md)
- Related: [map-config-and-image-selection](map-config-and-image-selection.md)
- Related: [practice-strict-config-parsing-and-no-silent-defaults](practice-strict-config-parsing-and-no-silent-defaults.md)
- Related: [practice-project-config-owns-rules](practice-project-config-owns-rules.md)
- Related: [map-cli-surface-and-exit-codes](../reporting/map-cli-surface-and-exit-codes.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:8
<!-- kk:citations:end -->
