---
type: map
title: Agent plugin catalogs and shared skill delivery
description: >-
  Nine shared skills for Codex, Cursor and Claude; Git catalogs, reproducible
  ZIPs, setup onboarding and separate CLI delivery.
tags:
  - plugins
  - agents
  - distribution
kk_schema_version: 3
kk_id: map-agent-plugin-catalogs-and-shared-skill-delivery
kk_derived_from:
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:7'
kk_relates_to:
  - map-component-boundaries-and-profile-contract
  - map-core-profile-checker-architecture
  - map-ci-workflow-triggers-and-jobs
  - practice-project-config-owns-rules
  - map-report-v1-json-contract
kk_depends_on: []
kk_confidence: high
---
The native catalogs `.agents/plugins/marketplace.json`, `.claude-plugin/marketplace.json` and `.cursor-plugin/marketplace.json` all resolve `internal/agentskills`. The marketplace is phpwarden-dev and the plugin is phpwarden. Its nine shared skills are phpwarden, phpwarden-setup, phpwarden-architecture, phpwarden-review, phpwarden-doctor, phpwarden-upgrade, phpwarden-performance, phpwarden-tests and phpwarden-report. The root skill routes the requested scope; specialist skills keep analyzer findings separate from agent interpretation.

`cmd/phpwarden-dist` packages deterministic client ZIPs and a repository-layout catalog ZIP with SHA256SUMS, build version and exact commit. Git development manifests omit a fixed version; versioned artifacts declare the build identity. Source catalogs and the copied canonical installer are checked for drift. Plugin packages include skills and shell bootstrap helpers, not CLI binaries, workers or automatically registered MCP servers/hooks.

Codex metadata points onboardingSkill to phpwarden-setup. Claude/Cursor invoke that skill explicitly; there is no universal install callback or required SessionStart hook. Updating a Git-sourced plugin refreshes its skills/helpers independently of installing/updating the CLI. A recommended CLI version is not a certified compatibility range.

`init --agents` and `--instructions` are optional project integrations independent of native plugin installation; avoid duplicating skills in one client. Public marketplace submission and actual native install/discovery/invocation/update/uninstall are separate from file/schema/package tests. Canonical delivery behavior is documented in plugins/README.md.

<!-- kk:related:start -->
# Related

- Related: [map-component-boundaries-and-profile-contract](map-component-boundaries-and-profile-contract.md)
- Related: [map-core-profile-checker-architecture](map-core-profile-checker-architecture.md)
- Related: [map-ci-workflow-triggers-and-jobs](../testing-ci/map-ci-workflow-triggers-and-jobs.md)
- Related: [practice-project-config-owns-rules](../configuration/practice-project-config-owns-rules.md)
- Related: [map-report-v1-json-contract](../reporting/map-report-v1-json-contract.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:7
<!-- kk:citations:end -->
