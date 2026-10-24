# kenkeep Index: checkers

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) to learn about: Checkers register a descriptor and run argv-only with validated native exit codes and report structure. #checker #process #adapters
- Open [**Criteria a new third-party tool must meet before becoming a checker**](practice-criteria-for-adding-a-new-checker.md) to learn about: New checkers need their own settings shape, license review, a defined diff scope and own verdict/completeness; opt-in advisory first. #checkers #integration #licensing
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) to learn about: Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph. #deptrac #checker #adapter
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) to learn about: Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need. #exakat #checkers #scope
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) to learn about: The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones. #jscpd #adapter #duplication
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) to learn about: Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned. #pins #deptrac #rector #docker
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) to learn about: Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure. #phpstan #adapter #diagnostics
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) to learn about: Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root. #rector #monorepo #composer-root

## Components (what exists)
- Open [**Deptrac bundle integration surface and supported project config**](map-deptrac-bundle-integration-surface.md) to learn about: How tools/deptrac drives Deptrac through its PHP APIs and which project config features it accepts or rejects. #deptrac #config #bundle
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) to learn about: The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded. #drupal-config #checker #yaml
- Open [**PHPWarden checker inventory and defaults**](map-checker-inventory.md) to learn about: Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites. #checkers #config #defaults
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) to learn about: tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules. #semgrep #bundle #licensing #rules

## By topic

### #adapter
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) — Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph.
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) — The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones.
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
### #checker
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) — Checkers register a descriptor and run argv-only with validated native exit codes and report structure.
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) — Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph.
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) — The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](../configuration/practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
### #deptrac
- Open [**Deptrac adapter aborts the report when any file fails to parse**](practice-deptrac-adapter-aborts-on-parser-failures.md) — Deptrac continues after parser errors; the adapter must abort on syntax-error events and verify the parsed AST graph.
- Open [**Deptrac bundle integration surface and supported project config**](map-deptrac-bundle-integration-surface.md) — How tools/deptrac drives Deptrac through its PHP APIs and which project config features it accepts or rejects.
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) — Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned.
### #bundle
- Open [**Deptrac bundle integration surface and supported project config**](map-deptrac-bundle-integration-surface.md) — How tools/deptrac drives Deptrac through its PHP APIs and which project config features it accepts or rejects.
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #config
- Open [**.phpwarden.yaml v1 fields and checker settings**](../configuration/map-project-config-v1-schema.md) — Fields of the strict v1 project config, including the shared per-checker settings block.
- Open [**Reject ambiguous config input instead of guessing**](../configuration/practice-strict-config-parsing-and-no-silent-defaults.md) — Config parsing is strict and never silently infers versions, ranges or base refs.
- Open [**PHPWarden checker inventory and defaults**](map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #licensing
- Open [**Criteria a new third-party tool must meet before becoming a checker**](practice-criteria-for-adding-a-new-checker.md) — New checkers need their own settings shape, license review, a defined diff scope and own verdict/completeness; opt-in advisory first.
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #rector
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) — Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned.
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) — Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root.
### #adapters
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) — Checkers register a descriptor and run argv-only with validated native exit codes and report structure.
### #composer-root
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) — Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root.
### #defaults
- Open [**PHPWarden checker inventory and defaults**](map-checker-inventory.md) — Checker IDs, which are opt-in, advisory, networked or config-dependent, and per-checker prerequisites.
### #diagnostics
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](../testing-ci/practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](../testing-ci/map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.
- Open [**Resolve the image before the container exists, never from inside it**](../runtime/practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #drupal-config
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) — The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded.
### #duplication
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) — The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones.
### #exakat
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
### #integration
- Open [**Criteria a new third-party tool must meet before becoming a checker**](practice-criteria-for-adding-a-new-checker.md) — New checkers need their own settings shape, license review, a defined diff scope and own verdict/completeness; opt-in advisory first.
### #jscpd
- Open [**JSCPD clones must come from consecutive reference windows**](practice-jscpd-matches-consecutive-reference-windows-only.md) — The adapter constrains JSCPD matching to consecutive windows; it never clamps ranges or drops invalid clones.
### #monorepo
- Open [**Rector runs from composer_root, not the repo root**](practice-rector-runs-from-composer-root.md) — Rector resolves installed packages from its working directory, so monorepo runs must start in composer_root.
### #phpstan
- Open [**Run image tools against the project's own vendor, never install dependencies**](../runtime/practice-analyze-with-project-dependencies.md) — Tools come from the image, but analyzers read the project's core, contrib and autoload; missing deps are reported, not auto-installed.
- Open [**PHPStan trait context suffix and non-ignorable errors**](practice-phpstan-trait-context-and-non-ignorable-errors.md) — Normalize only a validated trait context suffix; ignorable:false is a code finding, not an analyzer failure.
### #pins
- Open [**Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold**](practice-tool-version-pins-and-rationale.md) — Deptrac 4.5+ needs PHP 8.2 and Drupal Rector 1.1.3 conflicts with Rector 2.6.2+, so both stay pinned.
### #process
- Open [**Run checkers through the shared lifecycle with strict process handling**](practice-checker-lifecycle-and-process-execution.md) — Checkers register a descriptor and run argv-only with validated native exit codes and report structure.
### #rules
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #scope
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
- Open [**Compute scope once in Git and filter findings by kind**](../git-scope/practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
### #semgrep
- Open [**Semgrep CE engine bundle: preparation, invocation and supported rules**](map-semgrep-engine-bundle-and-rule-limits.md) — tools/semgrep/prepare.py builds a checksum-locked semgrep-core bundle; PHPWarden calls it directly and accepts only local PHP rules.
### #yaml
- Open [**Drupal module config checker: top-level uuid/_core rules**](map-drupal-config-checker-semantics.md) — The drupal-config checker flags top-level uuid/_core in module config only, whole-file, natively in Go; site exports are excluded.