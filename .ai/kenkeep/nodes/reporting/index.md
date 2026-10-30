# kenkeep Index: reporting

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Keep fail_on and required separate: findings policy vs. execution failure**](practice-separate-failure-policy-from-findings.md) to learn about: fail_on governs which findings block; a required checker that cannot run still fails and is never silently clean. #checkers #fail-on #severity
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) to learn about: Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false. #report #exit-codes #policy
- Open [**Keep analyzer progress focused on the user operation**](practice-keep-analyzer-progress-focused-on-the-user-operation.md) to learn about: Interactive progress names the current operation and elapsed time without exposing Docker as a stage detail. #cli #progress #usability

## Components (what exists)
- Open [**PHPWarden CLI commands and exit-code contract**](map-cli-surface-and-exit-codes.md) to learn about: Static checks, setup, diagnostics, updates, project tests and performance commands; scope rules and stable exit codes. #cli #exit-codes #ci
- Open [**Default text output: quality categories**](map-text-output-rule-groups-layout.md) to learn about: Normal text groups findings, execution failures and final check statuses by quality category; JSON and compact retain technical IDs. #cli #output #text-format
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) to learn about: Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules. #report #json #schema

## By topic

### #cli
- Open [**PHPWarden CLI commands and exit-code contract**](map-cli-surface-and-exit-codes.md) — Static checks, setup, diagnostics, updates, project tests and performance commands; scope rules and stable exit codes.
- Open [**Default text output: quality categories**](map-text-output-rule-groups-layout.md) — Normal text groups findings, execution failures and final check statuses by quality category; JSON and compact retain technical IDs.
- Open [**CLI bootstrap and per-project setup status**](../configuration/map-cli-bootstrap-and-per-project-setup-status.md) — Setup skill discovers managed or user CLI, installs verified artifacts on request and separates config, Docker and worker readiness.
### #exit-codes
- Open [**PHPWarden CLI commands and exit-code contract**](map-cli-surface-and-exit-codes.md) — Static checks, setup, diagnostics, updates, project tests and performance commands; scope rules and stable exit codes.
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) — Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false.
### #report
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) — Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false.
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
- Open [**Recorded SPX performance reader and MCP queries**](../runtime/map-recorded-spx-performance-reader-and-mcp-queries.md) — Bounded native SPX reader and five read-only MCP tools share verified capture snapshots without requiring a running app or Docker.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](../configuration/practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**PHPWarden checker inventory and defaults**](../checkers/map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
### #ci
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Path-filtered Checks, PR/manual Sandbox and Mutation, manual/tag Build targets and tag-only release publication.
- Open [**Resolve the image before the container exists, never from inside it**](../runtime/practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #fail-on
- Open [**Keep fail_on and required separate: findings policy vs. execution failure**](practice-separate-failure-policy-from-findings.md) — fail_on governs which findings block; a required checker that cannot run still fails and is never silently clean.
### #json
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
### #output
- Open [**Default text output: quality categories**](map-text-output-rule-groups-layout.md) — Normal text groups findings, execution failures and final check statuses by quality category; JSON and compact retain technical IDs.
### #policy
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) — Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false.
### #progress
- Open [**Keep analyzer progress focused on the user operation**](practice-keep-analyzer-progress-focused-on-the-user-operation.md) — Interactive progress names the current operation and elapsed time without exposing Docker as a stage detail.
### #schema
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
- Open [**.phpwarden.yaml v1 fields and checker settings**](../configuration/map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
### #severity
- Open [**Keep fail_on and required separate: findings policy vs. execution failure**](practice-separate-failure-policy-from-findings.md) — fail_on governs which findings block; a required checker that cannot run still fails and is never silently clean.
### #text-format
- Open [**Default text output: quality categories**](map-text-output-rule-groups-layout.md) — Normal text groups findings, execution failures and final check statuses by quality category; JSON and compact retain technical IDs.
### #usability
- Open [**Expose all project settings in the generated init config**](../configuration/practice-expose-all-project-settings-in-the-generated-init-config.md) — Init makes supported settings discoverable, keeping optional settings commented and existing project configuration intact.
- Open [**Keep analyzer progress focused on the user operation**](practice-keep-analyzer-progress-focused-on-the-user-operation.md) — Interactive progress names the current operation and elapsed time without exposing Docker as a stage detail.