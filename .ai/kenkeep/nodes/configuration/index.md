# kenkeep Index: configuration

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) to learn about: PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config. #settings #config #checkers
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) to learn about: Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters. #config #project #profile
- Open [**Expose all project settings in the generated init config**](practice-expose-all-project-settings-in-the-generated-init-config.md) to learn about: Init makes supported settings discoverable, keeping optional settings commented and existing project configuration intact. #config #init #usability
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) to learn about: Config parsing is strict and never silently infers versions, ranges or base refs. #config #validation
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) to learn about: Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1. #config #paths #precedence

## Components (what exists)
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) to learn about: Fields of the strict v1 project config, including the shared per-checker settings block. #config #schema
- Open [**php.runtime detection, image naming and monorepo composer_root**](map-config-and-image-selection.md) to learn about: Init detects php.runtime, which selects the per-PHP-version image; composer_root locates Drupal inside a monorepo. #config #docker #php-version

## By topic

### #config
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.
- Open [**PHPWarden checker inventory and defaults**](../checkers/map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](../checkers/practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.
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
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Report v1 JSON structure and evolution rules**](../reporting/map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
### #settings
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
### #usability
- Open [**Expose all project settings in the generated init config**](practice-expose-all-project-settings-in-the-generated-init-config.md) — Init makes supported settings discoverable, keeping optional settings commented and existing project configuration intact.
- Open [**Keep analyzer progress focused on the user operation**](../reporting/practice-keep-analyzer-progress-focused-on-the-user-operation.md) — Interactive progress names the current operation and elapsed time without exposing Docker as a stage detail.
### #validation
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.