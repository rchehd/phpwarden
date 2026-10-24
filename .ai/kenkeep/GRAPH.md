---
schema_version: 3
nodes_hash: 'sha256:9a2bc0e0901aecd958093059ad2f3e04d098e0f10646c75c9fa7f1e7c50626f1'
node_count: 45
---
# kenkeep Graph

Total nodes: 45

## map-checker-inventory

- **kind:** map
- **title:** PHPWarden checker inventory and defaults
- **path:** checkers/map-checker-inventory.md
- **tags:** checkers, config, defaults
- **relates_to:** practice-separate-failure-policy-from-findings, practice-keep-native-tool-options-in-native-configs, practice-analyze-with-project-dependencies
- **derived_from:** README.md, 01a1693d-cb80-7003-ae47-1468fd6435b0:map:1

## map-ci-workflow-triggers-and-jobs

- **kind:** map
- **title:** CI workflow triggers and jobs
- **path:** testing-ci/map-ci-workflow-triggers-and-jobs.md
- **tags:** ci, docker, release
- **relates_to:** practice-run-docker-integration-suites
- **derived_from:** README.md, 01a1693d-cb80-7003-ae47-1468fd6435b0:map:0

## map-cli-surface-and-exit-codes

- **kind:** map
- **title:** PHPWarden CLI commands and exit-code contract
- **path:** reporting/map-cli-surface-and-exit-codes.md
- **tags:** cli, exit-codes, ci
- **derived_from:** README.md

## map-component-boundaries-and-profile-contract

- **kind:** map
- **title:** Go single-module layout and profile contract
- **path:** architecture/map-component-boundaries-and-profile-contract.md
- **tags:** architecture, profile, go
- **derived_from:** docs/CORE-CONTRACTS.md

## map-config-and-image-selection

- **kind:** map
- **title:** php.runtime detection, image naming and monorepo composer_root
- **path:** configuration/map-config-and-image-selection.md
- **tags:** config, docker, php-version
- **derived_from:** README.md

## map-core-profile-checker-architecture

- **kind:** map
- **title:** Modular architecture: core, profiles, checkers, runners, integrations
- **path:** architecture/map-core-profile-checker-architecture.md
- **tags:** architecture, modules, profiles, checkers
- **derived_from:** docs/CONCEPT.md

## map-deptrac-bundle-integration-surface

- **kind:** map
- **title:** Deptrac bundle integration surface and supported project config
- **path:** checkers/map-deptrac-bundle-integration-surface.md
- **tags:** deptrac, config, bundle
- **derived_from:** tools/deptrac/README.md

## map-drupal-config-checker-semantics

- **kind:** map
- **title:** Drupal module config checker: top-level uuid/_core rules
- **path:** checkers/map-drupal-config-checker-semantics.md
- **tags:** drupal-config, checker, yaml
- **derived_from:** docs/FIRST-MILESTONE.md

## map-launcher-worker-execution-model

- **kind:** map
- **title:** Launcher and worker execution layers
- **path:** architecture/map-launcher-worker-execution-model.md
- **tags:** architecture, docker, launcher, worker
- **derived_from:** docs/CONCEPT.md

## map-profile-version-image-model

- **kind:** map
- **title:** Profile, framework version, PHP runtime/target, toolset and image model
- **path:** runtime/map-profile-version-image-model.md
- **tags:** profile, image, versions, docker
- **derived_from:** docs/CONCEPT.md

## map-project-config-v1-schema

- **kind:** map
- **title:** .phpwarden.yaml v1 fields and checker settings
- **path:** configuration/map-project-config-v1-schema.md
- **tags:** config, schema
- **derived_from:** docs/CORE-CONTRACTS.md

## map-report-v1-json-contract

- **kind:** map
- **title:** Report v1 JSON structure and evolution rules
- **path:** reporting/map-report-v1-json-contract.md
- **tags:** report, json, schema
- **derived_from:** docs/CORE-CONTRACTS.md

## map-result-cache

- **kind:** map
- **title:** PHPWarden result cache scope and keys
- **path:** runtime/map-result-cache.md
- **tags:** cache, runner, ci
- **derived_from:** README.md

## map-scenario-fixture-harness

- **kind:** map
- **title:** Scenario fixture harness layout
- **path:** testing-ci/map-scenario-fixture-harness.md
- **tags:** testing, fixtures, git
- **derived_from:** docs/TESTING.md

## map-semgrep-engine-bundle-and-rule-limits

- **kind:** map
- **title:** Semgrep CE engine bundle: preparation, invocation and supported rules
- **path:** checkers/map-semgrep-engine-bundle-and-rule-limits.md
- **tags:** semgrep, bundle, licensing, rules
- **derived_from:** tools/semgrep/README.md

## map-test-levels-and-acceptance-suites

- **kind:** map
- **title:** Test levels and acceptance suites
- **path:** testing-ci/map-test-levels-and-acceptance-suites.md
- **tags:** testing, docker, acceptance
- **derived_from:** docs/TESTING.md

## map-text-output-rule-groups-layout

- **kind:** map
- **title:** Default text output: quality categories
- **path:** reporting/map-text-output-rule-groups-layout.md
- **tags:** cli, output, text-format
- **relates_to:** practice-exit-codes-and-completeness-semantics, practice-separate-failure-policy-from-findings
- **derived_from:** docs/CLI-OUTPUT-IMPLEMENTATION-2026-10-24.md, 01a1693d-cb80-7003-ae47-1468fd6435b0:map:2

## practice-analyze-with-project-dependencies

- **kind:** practice
- **title:** Run image tools against the project's own vendor, never install dependencies
- **path:** runtime/practice-analyze-with-project-dependencies.md
- **tags:** dependencies, phpstan, docker
- **derived_from:** docs/CONCEPT.md

## practice-checker-lifecycle-and-process-execution

- **kind:** practice
- **title:** Run checkers through the shared lifecycle with strict process handling
- **path:** checkers/practice-checker-lifecycle-and-process-execution.md
- **tags:** checker, process, adapters
- **derived_from:** docs/CORE-CONTRACTS.md

## practice-checker-test-requirements

- **kind:** practice
- **title:** Cover every new checker with fixtures and failure tests
- **path:** testing-ci/practice-checker-test-requirements.md
- **tags:** testing, checkers, fixtures
- **derived_from:** docs/TESTING.md

## practice-config-root-paths-and-precedence

- **kind:** practice
- **title:** Resolve project root and paths, then layer config in fixed order
- **path:** configuration/practice-config-root-paths-and-precedence.md
- **tags:** config, paths, precedence
- **derived_from:** docs/CORE-CONTRACTS.md

## practice-criteria-for-adding-a-new-checker

- **kind:** practice
- **title:** Criteria a new third-party tool must meet before becoming a checker
- **path:** checkers/practice-criteria-for-adding-a-new-checker.md
- **tags:** checkers, integration, licensing
- **derived_from:** docs/TOOL-CANDIDATES.md

## practice-deptrac-adapter-aborts-on-parser-failures

- **kind:** practice
- **title:** Deptrac adapter aborts the report when any file fails to parse
- **path:** checkers/practice-deptrac-adapter-aborts-on-parser-failures.md
- **tags:** deptrac, checker, adapter
- **derived_from:** docs/VALIDATION-2026-10-23.md

## practice-diff-mode-scope-per-checker

- **kind:** practice
- **title:** Decide per checker whether diff mode reports the whole file or changed lines
- **path:** git-scope/practice-diff-mode-scope-per-checker.md
- **tags:** diff, scope, checkers
- **derived_from:** docs/TOOLS-SETTINGS.md

## practice-exakat-ce-deliberately-not-integrated

- **kind:** practice
- **title:** Exakat CE is deliberately not integrated into the image or default pipeline
- **path:** checkers/practice-exakat-ce-deliberately-not-integrated.md
- **tags:** exakat, checkers, scope
- **derived_from:** docs/TOOL-CANDIDATES.md

## practice-exit-codes-and-completeness-semantics

- **kind:** practice
- **title:** Map results to exit codes by fixed priority and track completeness
- **path:** reporting/practice-exit-codes-and-completeness-semantics.md
- **tags:** report, exit-codes, policy
- **derived_from:** docs/CORE-CONTRACTS.md

## practice-expose-all-project-settings-in-the-generated-init-config

- **kind:** practice
- **title:** Expose all project settings in the generated init config
- **path:** configuration/practice-expose-all-project-settings-in-the-generated-init-config.md
- **tags:** config, init, usability
- **relates_to:** practice-project-config-owns-rules, practice-keep-native-tool-options-in-native-configs
- **derived_from:** 01a1693d-cb80-7003-ae47-1468fd6435b0:practice:0

## practice-image-resolution-and-run-isolation

- **kind:** practice
- **title:** Select images by digest or ID and keep runs isolated and fingerprinted
- **path:** runtime/practice-image-resolution-and-run-isolation.md
- **tags:** runner, docker, images
- **derived_from:** docs/CORE-CONTRACTS.md

## practice-jscpd-matches-consecutive-reference-windows-only

- **kind:** practice
- **title:** JSCPD clones must come from consecutive reference windows
- **path:** checkers/practice-jscpd-matches-consecutive-reference-windows-only.md
- **tags:** jscpd, adapter, duplication
- **derived_from:** docs/BUGFIXES-2026-10-24.md

## practice-keep-analyzer-progress-focused-on-the-user-operation

- **kind:** practice
- **title:** Keep analyzer progress focused on the user operation
- **path:** reporting/practice-keep-analyzer-progress-focused-on-the-user-operation.md
- **tags:** cli, progress, usability
- **relates_to:** map-launcher-worker-execution-model, map-text-output-rule-groups-layout
- **derived_from:** 01a1693d-cb80-7003-ae47-1468fd6435b0:practice:2

## practice-keep-native-tool-options-in-native-configs

- **kind:** practice
- **title:** Expose only common options; leave detailed rules in native tool configs
- **path:** configuration/practice-keep-native-tool-options-in-native-configs.md
- **tags:** settings, config, checkers
- **derived_from:** docs/TOOLS-SETTINGS.md

## practice-let-projects-configure-analyzer-concurrency

- **kind:** practice
- **title:** Let projects configure analyzer concurrency
- **path:** runtime/practice-let-projects-configure-analyzer-concurrency.md
- **tags:** runner, config, concurrency
- **relates_to:** practice-project-config-owns-rules, practice-checker-lifecycle-and-process-execution
- **derived_from:** 01a1693d-cb80-7003-ae47-1468fd6435b0:practice:1

## practice-mount-read-only-apply-fixes-on-host

- **kind:** practice
- **title:** Keep project mounts read-only and apply fixes on the host
- **path:** runtime/practice-mount-read-only-apply-fixes-on-host.md
- **tags:** safety, fix, docker
- **derived_from:** README.md

## practice-phpstan-trait-context-and-non-ignorable-errors

- **kind:** practice
- **title:** PHPStan trait context suffix and non-ignorable errors
- **path:** checkers/practice-phpstan-trait-context-and-non-ignorable-errors.md
- **tags:** phpstan, adapter, diagnostics
- **derived_from:** docs/BUGFIXES-2026-10-24.md

## practice-pre-commit-hook-analysis-reads-working-tree-context

- **kind:** practice
- **title:** Pre-commit hook analyzes staged files against working-tree context
- **path:** git-scope/practice-pre-commit-hook-analysis-reads-working-tree-context.md
- **tags:** pre-commit, hook, git, scope
- **derived_from:** docs/PRE-COMMIT-2026-10-24.md

## practice-pre-commit-hook-checks-staged-content

- **kind:** practice
- **title:** Pre-commit hook checks staged files and never rewrites the tree
- **path:** git-scope/practice-pre-commit-hook-checks-staged-content.md
- **tags:** git-hook, safety, pre-commit
- **derived_from:** README.md

## practice-project-config-owns-rules

- **kind:** practice
- **title:** Store profile and rules in the project's .phpwarden.yaml, not account settings
- **path:** configuration/practice-project-config-owns-rules.md
- **tags:** config, project, profile
- **derived_from:** docs/CONCEPT.md

## practice-rector-runs-from-composer-root

- **kind:** practice
- **title:** Rector runs from composer_root, not the repo root
- **path:** checkers/practice-rector-runs-from-composer-root.md
- **tags:** rector, monorepo, composer-root
- **derived_from:** docs/BUGFIXES-2026-10-24.md

## practice-run-docker-integration-suites

- **kind:** practice
- **title:** Build all PHP images before running Docker acceptance suites
- **path:** testing-ci/practice-run-docker-integration-suites.md
- **tags:** testing, docker, ci
- **derived_from:** docs/TESTING.md

## practice-run-python-and-go-test-suites

- **kind:** practice
- **title:** Run the Python harness and Go suites from the repo root
- **path:** testing-ci/practice-run-python-and-go-test-suites.md
- **tags:** testing, ci, commands
- **derived_from:** docs/TESTING.md

## practice-scope-computation-and-reporting-rules

- **kind:** practice
- **title:** Compute scope once in Git and filter findings by kind
- **path:** git-scope/practice-scope-computation-and-reporting-rules.md
- **tags:** scope, git, findings
- **derived_from:** docs/CORE-CONTRACTS.md

## practice-select-image-before-container-start

- **kind:** practice
- **title:** Resolve the image before the container exists, never from inside it
- **path:** runtime/practice-select-image-before-container-start.md
- **tags:** docker, ci, image, runtime
- **derived_from:** docs/CONCEPT.md

## practice-separate-failure-policy-from-findings

- **kind:** practice
- **title:** Keep fail_on and required separate: findings policy vs. execution failure
- **path:** reporting/practice-separate-failure-policy-from-findings.md
- **tags:** checkers, fail-on, severity
- **derived_from:** docs/TOOLS-SETTINGS.md

## practice-strict-config-parsing-and-no-silent-defaults

- **kind:** practice
- **title:** Reject ambiguous config input instead of guessing
- **path:** configuration/practice-strict-config-parsing-and-no-silent-defaults.md
- **tags:** config, validation
- **derived_from:** docs/CORE-CONTRACTS.md

## practice-tool-version-pins-and-rationale

- **kind:** practice
- **title:** Keep Deptrac at 4.4.0 and Rector at 2.6.1 while their constraints hold
- **path:** checkers/practice-tool-version-pins-and-rationale.md
- **tags:** pins, deptrac, rector, docker
- **derived_from:** README.md
