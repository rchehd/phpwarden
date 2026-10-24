---
okf_version: '0.1'
---
# kenkeep Index

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) to learn about: Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI. #testing #docker #ci
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) to learn about: Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind. #scope #git #findings
- Open [**Cover every new checker with fixtures and failure tests**](practice-checker-test-requirements.md) to learn about: Each checker needs clean and failing fixtures, setting-driven cases, failure injection, and real-tool integration. #testing #checkers #fixtures
- Open [**Criteria a new third-party tool must meet before becoming a checker**](practice-criteria-for-adding-a-new-checker.md) to learn about: New checkers need their own settings shape, license review, a defined diff scope and own verdict/completeness; opt-in advisory first. #checkers #integration #licensing
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) to learn about: Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines. #diff #scope #checkers
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) to learn about: Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph. #deptrac #checker #adapter
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) to learn about: Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need. #exakat #checkers #scope
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) to learn about: PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config. #settings #config #checkers
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) to learn about: The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones. #jscpd #adapter #duplication
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) to learn about: Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned. #pins #deptrac #rector #docker
- Open [**Keep fail_on and required separate: findings policy vs. execution failure**](practice-separate-failure-policy-from-findings.md) to learn about: fail_on governs which findings block; a required checker that cannot run still fails and is never silently clean. #checkers #fail-on #severity
- Open [**Keep project mounts read-only and apply fixes on the host**](practice-mount-read-only-apply-fixes-on-host.md) to learn about: Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host. #safety #fix #docker
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) to learn about: Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false. #report #exit-codes #policy
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) to learn about: Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure. #phpstan #adapter #diagnostics
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) to learn about: The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export. #pre-commit #hook #git #scope
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) to learn about: Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages. #git-hook #safety #pre-commit
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) to learn about: Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root. #rector #monorepo #composer-root
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) to learn about: Config parsing is strict and never silently infers versions, ranges or base refs. #config #validation
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) to learn about: Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1. #config #paths #precedence
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) to learn about: PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image). #docker #ci #image #runtime
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) to learn about: Checkers register a descriptor and run argv-only with validated native exit codes and report structure. #checker #process #adapters
- Open [**Run image tools against the project's own vendor, never install dependencies**](practice-analyze-with-project-dependencies.md) to learn about: Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed. #dependencies #phpstan #docker
- Open [**Run the Python harness and Go suites from the repo root**](practice-run-python-and-go-test-suites.md) to learn about: Exact commands for Python harness checks, Go vet/race tests, bundle installs and native worker acceptance. #testing #ci #commands
- Open [**Select images by digest or ID and keep runs isolated and fingerprinted**](practice-image-resolution-and-run-isolation.md) to learn about: Image selection is offline and exact, runs are read-only and sandboxed, and drift during a run makes results stale. #runner #docker #images
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) to learn about: Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters. #config #project #profile

## Components (what exists)
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) to learn about: Fields of the strict v1 project config, including the shared per-checker settings block. #config #schema
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) to learn about: Go and fixture jobs run on every push/PR; Docker builds, image publishing and CLI artifacts only on manual runs or v* tags. #ci #docker #release
- Open [**Default text output: rule-grouped findings layout**](map-text-output-rule-groups-layout.md) to learn about: Text report leads with verdict and totals, then groups findings by severity+checker+rule, capped per group unless --all. #cli #output #text-format
- Open [**Deptrac bundle integration surface and supported project config**](map-deptrac-bundle-integration-surface.md) to learn about: How tools/deptrac drives Deptrac through its PHP APIs and which project config features it accepts or rejects. #deptrac #config #bundle
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) to learn about: The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded. #drupal-config #checker #yaml
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) to learn about: One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic. #architecture #profile #go
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) to learn about: PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks. #architecture #docker #launcher #worker
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) to learn about: Core components, the profile and checker layers, and the rule that core stays framework-agnostic. #architecture #modules #profiles #checkers
- Open [**php.runtime detection, image naming and monorepo composer_root**](map-config-and-image-selection.md) to learn about: Init detects php.runtime, which selects the per-PHP-version image; composer_root locates Drupal inside a monorepo. #config #docker #php-version
- Open [**PHPWarden checker inventory and defaults**](map-checker-inventory.md) to learn about: Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites. #checkers #config #defaults
- Open [**PHPWarden CLI commands and exit-code contract**](map-cli-surface-and-exit-codes.md) to learn about: Commands init, check, fix, scope, config validate/show, hook pre-commit; exit codes 0/1/2/3/130 are stable for CI. #cli #exit-codes #ci
- Open [**PHPWarden result cache scope and keys**](map-result-cache.md) to learn about: Opt-in cache of complete file-only check results, keyed on content, config, toolset and image digest, 24h TTL. #cache #runner #ci
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) to learn about: One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image. #profile #image #versions #docker
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) to learn about: Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules. #report #json #schema
- Open [**Scenario fixture harness layout**](map-scenario-fixture-harness.md) to learn about: JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness. #testing #fixtures #git
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) to learn about: tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules. #semgrep #bundle #licensing #rules
- Open [**Test levels and acceptance suites**](map-test-levels-and-acceptance-suites.md) to learn about: Four test levels from repo quality to real tools in images, plus the worker, Docker and launcher acceptance modules. #testing #docker #acceptance

## By topic

### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) — Go and fixture jobs run on every push/PR; Docker builds, image publishing and CLI artifacts only on manual runs or v* tags.
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #checkers
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
### #config
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.
- Open [**PHPWarden checker inventory and defaults**](map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #ci
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) — Go and fixture jobs run on every push/PR; Docker builds, image publishing and CLI artifacts only on manual runs or v* tags.
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #testing
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**Run the Python harness and Go suites from the repo root**](practice-run-python-and-go-test-suites.md) — Exact commands for Python harness checks, Go vet/race tests, bundle installs and native worker acceptance.
- Open [**Test levels and acceptance suites**](map-test-levels-and-acceptance-suites.md) — Four test levels from repo quality to real tools in images, plus the worker, Docker and launcher acceptance modules.
### #scope
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
### #adapter
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) — Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph.
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) — The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones.
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
### #architecture
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) — PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks.
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) — Core components, the profile and checker layers, and the rule that core stays framework-agnostic.
### #checker
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) — Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph.
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) — The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded.
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) — Checkers register a descriptor and run argv-only with validated native exit codes and report structure.
### #deptrac
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) — Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph.
- Open [**Deptrac bundle integration surface and supported project config**](map-deptrac-bundle-integration-surface.md) — How tools/deptrac drives Deptrac through its PHP APIs and which project config features it accepts or rejects.
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) — Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned.
### #git
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
- Open [**Scenario fixture harness layout**](map-scenario-fixture-harness.md) — JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness.
### #profile
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) — Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters.
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
### #bundle
- Open [**Deptrac bundle integration surface and supported project config**](map-deptrac-bundle-integration-surface.md) — How tools/deptrac drives Deptrac through its PHP APIs and which project config features it accepts or rejects.
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #cli
- Open [**Default text output: rule-grouped findings layout**](map-text-output-rule-groups-layout.md) — Text report leads with verdict and totals, then groups findings by severity+checker+rule, capped per group unless --all.
- Open [**PHPWarden CLI commands and exit-code contract**](map-cli-surface-and-exit-codes.md) — Commands init, check, fix, scope, config validate/show, hook pre-commit; exit codes 0/1/2/3/130 are stable for CI.
### #exit-codes
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) — Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false.
- Open [**PHPWarden CLI commands and exit-code contract**](map-cli-surface-and-exit-codes.md) — Commands init, check, fix, scope, config validate/show, hook pre-commit; exit codes 0/1/2/3/130 are stable for CI.
### #fixtures
- Open [**Cover every new checker with fixtures and failure tests**](practice-checker-test-requirements.md) — Each checker needs clean and failing fixtures, setting-driven cases, failure injection, and real-tool integration.
- Open [**Scenario fixture harness layout**](map-scenario-fixture-harness.md) — JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness.
### #image
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #licensing
- Open [**Criteria a new third-party tool must meet before becoming a checker**](practice-criteria-for-adding-a-new-checker.md) — New checkers need their own settings shape, license review, a defined diff scope and own verdict/completeness; opt-in advisory first.
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #phpstan
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
- Open [**Run image tools against the project's own vendor, never install dependencies**](practice-analyze-with-project-dependencies.md) — Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed.
### #pre-commit
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.
### #rector
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) — Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned.
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) — Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root.
### #report
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) — Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false.
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
### #runner
- Open [**PHPWarden result cache scope and keys**](map-result-cache.md) — Opt-in cache of complete file-only check results, keyed on content, config, toolset and image digest, 24h TTL.
- Open [**Select images by digest or ID and keep runs isolated and fingerprinted**](practice-image-resolution-and-run-isolation.md) — Image selection is offline and exact, runs are read-only and sandboxed, and drift during a run makes results stale.
### #safety
- Open [**Keep project mounts read-only and apply fixes on the host**](practice-mount-read-only-apply-fixes-on-host.md) — Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host.
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.
### #schema
- Open [**.phpwarden.yaml v1 fields and checker settings**](map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
### #acceptance
- Open [**Test levels and acceptance suites**](map-test-levels-and-acceptance-suites.md) — Four test levels from repo quality to real tools in images, plus the worker, Docker and launcher acceptance modules.
### #adapters
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) — Checkers register a descriptor and run argv-only with validated native exit codes and report structure.
### #cache
- Open [**PHPWarden result cache scope and keys**](map-result-cache.md) — Opt-in cache of complete file-only check results, keyed on content, config, toolset and image digest, 24h TTL.
### #commands
- Open [**Run the Python harness and Go suites from the repo root**](practice-run-python-and-go-test-suites.md) — Exact commands for Python harness checks, Go vet/race tests, bundle installs and native worker acceptance.
### #composer-root
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) — Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root.
### #defaults
- Open [**PHPWarden checker inventory and defaults**](map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #dependencies
- Open [**Run image tools against the project's own vendor, never install dependencies**](practice-analyze-with-project-dependencies.md) — Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed.
### #diagnostics
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
### #diff
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
### #drupal-config
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) — The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded.
### #duplication
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) — The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones.
### #exakat
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
### #fail-on
- Open [**Keep fail_on and required separate: findings policy vs. execution failure**](practice-separate-failure-policy-from-findings.md) — fail_on governs which findings block; a required checker that cannot run still fails and is never silently clean.
### #findings
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
### #fix
- Open [**Keep project mounts read-only and apply fixes on the host**](practice-mount-read-only-apply-fixes-on-host.md) — Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host.
### #git-hook
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.
### #go
- Open [**Go single-module layout and profile contract**](map-component-boundaries-and-profile-contract.md) — One Go module holds launcher, worker and shared packages; profiles own framework knowledge, core stays framework-agnostic.
### #hook
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
### #images
- Open [**Select images by digest or ID and keep runs isolated and fingerprinted**](practice-image-resolution-and-run-isolation.md) — Image selection is offline and exact, runs are read-only and sandboxed, and drift during a run makes results stale.
### #integration
- Open [**Criteria a new third-party tool must meet before becoming a checker**](practice-criteria-for-adding-a-new-checker.md) — New checkers need their own settings shape, license review, a defined diff scope and own verdict/completeness; opt-in advisory first.
### #jscpd
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) — The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones.
### #json
- Open [**Report v1 JSON structure and evolution rules**](map-report-v1-json-contract.md) — Shape of the check report v1: top-level keys, finding and diagnostic fields, ordering and compatibility rules.
### #launcher
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) — PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks.
### #modules
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) — Core components, the profile and checker layers, and the rule that core stays framework-agnostic.
### #monorepo
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) — Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root.
### #output
- Open [**Default text output: rule-grouped findings layout**](map-text-output-rule-groups-layout.md) — Text report leads with verdict and totals, then groups findings by severity+checker+rule, capped per group unless --all.
### #paths
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) — Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1.
### #php-version
- Open [**php.runtime detection, image naming and monorepo composer_root**](map-config-and-image-selection.md) — Init detects php.runtime, which selects the per-PHP-version image; composer_root locates Drupal inside a monorepo.
### #pins
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) — Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned.
### #policy
- Open [**Map results to exit codes by fixed priority and track completeness**](practice-exit-codes-and-completeness-semantics.md) — Exit 0/1/2/3/130 follow a strict priority; optional failures keep exit 0 but set complete=false.
### #precedence
- Open [**Resolve project root and paths, then layer config in fixed order**](practice-config-root-paths-and-precedence.md) — Fixed root lookup, relative-path bases and override order; no account-wide or local config layers in v1.
### #process
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) — Checkers register a descriptor and run argv-only with validated native exit codes and report structure.
### #profiles
- Open [**Modular architecture: core, profiles, checkers, runners, integrations**](map-core-profile-checker-architecture.md) — Core components, the profile and checker layers, and the rule that core stays framework-agnostic.
### #project
- Open [**Store profile and rules in the project's .phpwarden.yaml, not account settings**](practice-project-config-owns-rules.md) — Profile, PHP target, rules and blocking policy live in the project-root config; native client settings are only adapters.
### #release
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) — Go and fixture jobs run on every push/PR; Docker builds, image publishing and CLI artifacts only on manual runs or v* tags.
### #rules
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #runtime
- Open [**Resolve the image before the container exists, never from inside it**](practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #semgrep
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #settings
- Open [**Expose only common options; leave detailed rules in native tool configs**](practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
### #severity
- Open [**Keep fail_on and required separate: findings policy vs. execution failure**](practice-separate-failure-policy-from-findings.md) — fail_on governs which findings block; a required checker that cannot run still fails and is never silently clean.
### #text-format
- Open [**Default text output: rule-grouped findings layout**](map-text-output-rule-groups-layout.md) — Text report leads with verdict and totals, then groups findings by severity+checker+rule, capped per group unless --all.
### #validation
- Open [**Reject ambiguous config input instead of guessing**](practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.
### #versions
- Open [**Profile, framework version, PHP runtime/target, toolset and image model**](map-profile-version-image-model.md) — One profile per framework; versions, runtime and tool sets are resolved via a compatibility table to a digest-pinned image.
### #worker
- Open [**Launcher and worker execution layers**](map-launcher-worker-execution-model.md) — PHPWarden splits execution into a host-side Go launcher and an in-image Go worker that runs the checks.
### #yaml
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) — The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded.
