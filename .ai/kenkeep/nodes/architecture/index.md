# kenkeep Index: architecture

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
_None yet._

## Components (what exists)
- Open [**Agent plugin catalogs and shared skill delivery**](map-agent-plugin-catalogs-and-shared-skill-delivery.md) to learn about: Nine shared skills for Codex, Cursor and Claude; Git catalogs, reproducible ZIPs, setup onboarding and separate CLI delivery. #plugins #agents #distribution
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) to learn about: PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks. #architecture #docker #launcher #worker
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) to learn about: One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic. #architecture #profile #go
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) to learn about: Core components, the profile and checker layers, and the rule that core stays framework-agnostic. #architecture #modules #profiles #checkers

## By topic

### #architecture
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) — PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks.
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) — Core components, the profile and checker layers, and the rule that core stays framework-agnostic.
### #agents
- Open [**Agent plugin catalogs and shared skill delivery**](map-agent-plugin-catalogs-and-shared-skill-delivery.md) — Nine shared skills for Codex, Cursor and Claude; Git catalogs, reproducible ZIPs, setup onboarding and separate CLI delivery.
- Open [**CLI bootstrap and per-project setup status**](../configuration/map-cli-bootstrap-and-per-project-setup-status.md) — Setup skill discovers managed or user CLI, installs verified artifacts on request and separates config, Docker and worker readiness.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](../configuration/practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**PHPWarden checker inventory and defaults**](../checkers/map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
### #distribution
- Open [**Agent plugin catalogs and shared skill delivery**](map-agent-plugin-catalogs-and-shared-skill-delivery.md) — Nine shared skills for Codex, Cursor and Claude; Git catalogs, reproducible ZIPs, setup onboarding and separate CLI delivery.
### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Path-filtered Checks, PR/manual Sandbox and Mutation, manual/tag Build targets and tag-only release publication.
- Open [**Resolve the image before the container exists, never from inside it**](../runtime/practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #go
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
### #launcher
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) — PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks.
### #modules
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) — Core components, the profile and checker layers, and the rule that core stays framework-agnostic.
### #plugins
- Open [**Agent plugin catalogs and shared skill delivery**](map-agent-plugin-catalogs-and-shared-skill-delivery.md) — Nine shared skills for Codex, Cursor and Claude; Git catalogs, reproducible ZIPs, setup onboarding and separate CLI delivery.
### #profile
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](../configuration/practice-project-config-owns-rules.md) — Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters.
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](../runtime/map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
### #profiles
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) — Core components, the profile and checker layers, and the rule that core stays framework-agnostic.
### #worker
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) — PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks.