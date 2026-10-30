---
type: map
title: Recorded SPX performance reader and MCP queries
description: >-
  Bounded native SPX reader and five read-only MCP tools share verified capture
  snapshots without requiring a running app or Docker.
tags:
  - performance
  - mcp
  - report
kk_schema_version: 3
kk_id: map-recorded-spx-performance-reader-and-mcp-queries
kk_derived_from:
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:9'
kk_relates_to:
  - map-cli-surface-and-exit-codes
  - map-report-v1-json-contract
  - map-agent-plugin-catalogs-and-shared-skill-delivery
kk_depends_on: []
kk_confidence: high
---
`internal/performance` reads complete unsampled SPX captures through phpwarden performance --reports DIR --producer COMMIT; `internal/perfmcp` exposes the same reader through performance mcp. The producer is an explicit operator format assertion, not authenticated provenance. Capture validation, input fingerprints, size/time limits, rejected symlinks and completeness diagnostics apply before focused query results. Existing reports need no PHP, Docker, network or running application.

The five read-only MCP tools are find_reports, get_report_metadata, get_flat_profile, get_callers and get_aggregated_call_graph. They provide bounded views/pagination and snapshot identity; query arguments cannot select arbitrary files or execute commands. Flat timings cover the capture; the graph counts direct calls. Self time excludes children, total time includes children, and peak memory belongs to the whole capture. Source mappings identify definitions but do not establish source freshness or a regression.

Incomplete/malformed/sampled/changed captures return exit 3; bad arguments 2 and interruption 130. A complete read returns 0, through a separate result family from static checks. Reports omit sensitive metadata and URL query/userinfo, but request paths and function names still require review before sharing. The performance skill reads existing captures by default and routes explicit connection requests separately; neither a review nor plugin installation activates profiling.

<!-- kk:related:start -->
# Related

- Related: [map-cli-surface-and-exit-codes](../reporting/map-cli-surface-and-exit-codes.md)
- Related: [map-report-v1-json-contract](../reporting/map-report-v1-json-contract.md)
- Related: [map-agent-plugin-catalogs-and-shared-skill-delivery](../architecture/map-agent-plugin-catalogs-and-shared-skill-delivery.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:9
<!-- kk:citations:end -->
