---
type: map
title: Local SPX runtime and project MCP connections
description: >-
  Opt-in profiler adapters for DDEV, Compose and Docker preserve tracked project
  files and keep private state outside the checkout.
tags:
  - performance
  - mcp
  - docker
kk_schema_version: 3
kk_id: map-local-spx-runtime-and-project-mcp-connections
kk_derived_from:
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:10'
kk_relates_to:
  - map-recorded-spx-performance-reader-and-mcp-queries
  - practice-project-config-owns-rules
  - practice-mount-read-only-apply-fixes-on-host
  - map-agent-plugin-catalogs-and-shared-skill-delivery
kk_depends_on: []
kk_confidence: high
---
`internal/perfruntime` implements performance setup/status/teardown and connect/disconnect. Runtime management uses a local Docker Unix socket and a pinned, checksum-verified SPX source. Setup without --apply prepares an integration; explicit --apply can build a derivative and activate it. DDEV uses locally excluded adapter files, while Compose keeps overlays/build contexts outside the repository. Tracked runtime configuration, .gitignore and .phpwarden.yaml are preserved; private state/reports live outside the checkout and linked worktrees are distinct.

DDEV --apply restarts the selected project with hooks skipped. Compose preserves file/env/profile ordering and original service settings, resolves an existing image ID, and activates only the selected PHP service. Bare Docker --apply returns launch arguments without cloning/recreating the application container; launch and restoration remain explicit manual operations. Runtime support is Linux amd64 NTS PHP 8.1-8.4 with the documented build prerequisites.

Profiling requires explicit recording, with HTTP additionally opt-in and protected by a private key and local allowlist. Status probes do not start containers or claim HTTP/FPM acceptance. Teardown verifies ownership hashes and preserves edited adapters/captures; without --apply it does not unload an already running extension.

Connect uses the saved project runtime and host CLI to add only owned project-scoped client entries for Codex/Cursor/Claude, preserving other settings and servers. It does not install SPX or record a request. Disconnect removes unchanged owned entries, preserving captures and conflicts. The performance skill routes these native commands; configured connections and tools actually available in the live client are reported separately. No global MCP registration or plugin lifecycle hook is implied.

<!-- kk:related:start -->
# Related

- Related: [map-recorded-spx-performance-reader-and-mcp-queries](map-recorded-spx-performance-reader-and-mcp-queries.md)
- Related: [practice-project-config-owns-rules](../configuration/practice-project-config-owns-rules.md)
- Related: [practice-mount-read-only-apply-fixes-on-host](practice-mount-read-only-apply-fixes-on-host.md)
- Related: [map-agent-plugin-catalogs-and-shared-skill-delivery](../architecture/map-agent-plugin-catalogs-and-shared-skill-delivery.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:10
<!-- kk:citations:end -->
