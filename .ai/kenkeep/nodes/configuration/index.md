# kenkeep Index: configuration

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) to learn about: Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters. #config #project #profile
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) to learn about: PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config. #settings #config #checkers
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) to learn about: Config parsing is strict and never silently infers versions, ranges or base refs. #config #validation
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) to learn about: Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1. #config #paths #precedence
- Open [**Expose all project settings in the generated init config**](practice-expose-all-project-settings-in-the-generated-init-config.md) to learn about: Init makes supported settings discoverable, keeping optional settings commented and existing project configuration intact. #config #init #usability

## Components (what exists)
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) to learn about: Fields of the strict v1 project config, including the shared per-checker settings block. #config #schema
- Open [**php.runtime detection, image naming and monorepo composer_root**](map-config-and-image-selection.md) to learn about: Init detects php.runtime, which selects the per-PHP-version image; composer_root locates Drupal inside a monorepo. #config #docker #php-version
- Open [**CLI bootstrap and per-project setup status**](map-cli-bootstrap-and-per-project-setup-status.md) to learn about: Setup skill discovers managed or user CLI, installs verified artifacts on request and separates config, Docker and worker readiness. #config #agents #cli

## By topic

### #config
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.
- Open [**PHPWarden checker inventory and defaults**](../checkers/map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #agents
- Open [**Agent plugin catalogs and shared skill delivery**](../architecture/map-agent-plugin-catalogs-and-shared-skill-delivery.md) — Nine shared skills for Codex, Cursor and Claude; Git catalogs, reproducible ZIPs, setup onboarding and separate CLI delivery.
- Open [**CLI bootstrap and per-project setup status**](map-cli-bootstrap-and-per-project-setup-status.md) — Setup skill discovers managed or user CLI, installs verified artifacts on request and separates config, Docker and worker readiness.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**PHPWarden checker inventory and defaults**](../checkers/map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
### #cli
- Open [**PHPWarden CLI commands and exit-code contract**](../reporting/map-cli-surface-and-exit-codes.md) — Static checks, setup, diagnostics, updates, project tests and performance commands; scope rules and stable exit codes.
- Open [**Default text output: quality categories**](../reporting/map-text-output-rule-groups-layout.md) — Normal text groups findings, execution failures and final check statuses by quality category; JSON and compact retain technical IDs.
- Open [**CLI bootstrap and per-project setup status**](map-cli-bootstrap-and-per-project-setup-status.md) — Setup skill discovers managed or user CLI, installs verified artifacts on request and separates config, Docker and worker readiness.
### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Path-filtered Checks, PR/manual Sandbox and Mutation, manual/tag Build targets and tag-only release publication.
- Open [**Resolve the image before the container exists, never from inside it**](../runtime/practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #init
- Open [**Expose all project settings in the generated init config**](practice-expose-all-project-settings-in-the-generated-init-config.md) — Init makes supported settings discoverable, keeping optional settings commented and existing project configuration intact.
### #paths
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) — Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1.
### #php-version
- Open [**php.runtime detection, image naming and monorepo composer_root**](map-config-and-image-selection.md) — Init detects php.runtime, which selects the per-PHP-version image; composer_root locates Drupal inside a monorepo.
### #precedence
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) — Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1.
### #profile
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) — Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters.
- Open [**Go single-module layout and profile contract**](../architecture/map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](../runtime/map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
### #project
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) — Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters.
### #schema
- Open [**Report v1 JSON structure and evolution rules**](../reporting/map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
### #settings
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
### #usability
- Open [**Expose all project settings in the generated init config**](practice-expose-all-project-settings-in-the-generated-init-config.md) — Init makes supported settings discoverable, keeping optional settings commented and existing project configuration intact.
- Open [**Keep analyzer progress focused on the user operation**](../reporting/practice-keep-analyzer-progress-focused-on-the-user-operation.md) — Interactive progress names the current operation and elapsed time without exposing Docker as a stage detail.
### #validation
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.