# kenkeep Index: runtime

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) to learn about: PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image). #docker #ci #image #runtime
- Open [**Run image tools against the project's own vendor, never install dependencies**](practice-analyze-with-project-dependencies.md) to learn about: Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed. #dependencies #phpstan #docker
- Open [**Select images by digest or ID and keep runs isolated and fingerprinted**](practice-image-resolution-and-run-isolation.md) to learn about: Image selection is offline and exact, runs are read-only and sandboxed, and drift during a run makes results stale. #runner #docker #images
- Open [**Keep project mounts read-only and apply fixes on the host**](practice-mount-read-only-apply-fixes-on-host.md) to learn about: Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host. #safety #fix #docker
- Open [**Let projects configure analyzer concurrency**](practice-let-projects-configure-analyzer-concurrency.md) to learn about: Projects choose sequential, bounded or unrestricted worker concurrency according to available resources. #runner #config #concurrency

## Components (what exists)
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) to learn about: One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image. #profile #image #versions #docker
- Open [**PHPWarden result cache scope and keys**](map-result-cache.md) to learn about: Opt-in cache of complete file-only check results, keyed on content, config, toolset and image digest, 24h TTL. #cache #runner #ci
- Open [**Shared development toolset compatibility metadata**](map-shared-development-toolset-compatibility-metadata.md) to learn about: docker-bake.json supplies the shared development toolset revision to the CLI, manifest entries and Docker image labels. #toolset #docker #build

## By topic

### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #runner
- Open [**Select images by digest or ID and keep runs isolated and fingerprinted**](practice-image-resolution-and-run-isolation.md) — Image selection is offline and exact, runs are read-only and sandboxed, and drift during a run makes results stale.
- Open [**Let projects configure analyzer concurrency**](practice-let-projects-configure-analyzer-concurrency.md) — Projects choose sequential, bounded or unrestricted worker concurrency according to available resources.
- Open [**PHPWarden result cache scope and keys**](map-result-cache.md) — Opt-in cache of complete file-only check results, keyed on content, config, toolset and image digest, 24h TTL.
### #ci
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #image
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #build
- Open [**Shared development toolset compatibility metadata**](map-shared-development-toolset-compatibility-metadata.md) — docker-bake.json supplies the shared development toolset revision to the CLI, manifest entries and Docker image labels.
### #cache
- Open [**PHPWarden result cache scope and keys**](map-result-cache.md) — Opt-in cache of complete file-only check results, keyed on content, config, toolset and image digest, 24h TTL.
### #concurrency
- Open [**Let projects configure analyzer concurrency**](practice-let-projects-configure-analyzer-concurrency.md) — Projects choose sequential, bounded or unrestricted worker concurrency according to available resources.
### #config
- Open [**.phpwarden.yaml v1 fields and checker settings**](../configuration/map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Reject ambiguous config input instead of guessing**](../configuration/practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.
- Open [**PHPWarden checker inventory and defaults**](../checkers/map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #dependencies
- Open [**Run image tools against the project's own vendor, never install dependencies**](practice-analyze-with-project-dependencies.md) — Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed.
### #fix
- Open [**Keep project mounts read-only and apply fixes on the host**](practice-mount-read-only-apply-fixes-on-host.md) — Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host.
### #images
- Open [**Select images by digest or ID and keep runs isolated and fingerprinted**](practice-image-resolution-and-run-isolation.md) — Image selection is offline and exact, runs are read-only and sandboxed, and drift during a run makes results stale.
### #phpstan
- Open [**Run image tools against the project's own vendor, never install dependencies**](practice-analyze-with-project-dependencies.md) — Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed.
- Open [**PHPStan trait context suffix and non-ignorable errors**](../checkers/practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
### #profile
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](../configuration/practice-project-config-owns-rules.md) — Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters.
- Open [**Go single-module layout and profile contract**](../architecture/map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
### #runtime
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #safety
- Open [**Keep project mounts read-only and apply fixes on the host**](practice-mount-read-only-apply-fixes-on-host.md) — Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host.
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](../git-scope/practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.
### #toolset
- Open [**Shared development toolset compatibility metadata**](map-shared-development-toolset-compatibility-metadata.md) — docker-bake.json supplies the shared development toolset revision to the CLI, manifest entries and Docker image labels.
### #versions
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.