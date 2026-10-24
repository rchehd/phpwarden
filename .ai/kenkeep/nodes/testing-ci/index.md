# kenkeep Index: testing-ci

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) to learn about: Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI. #testing #docker #ci
- Open [**Cover every new checker with fixtures and failure tests**](practice-checker-test-requirements.md) to learn about: Each checker needs clean and failing fixtures, setting-driven cases, failure injection, and real-tool integration. #testing #checkers #fixtures
- Open [**Run the Python harness and Go suites from the repo root**](practice-run-python-and-go-test-suites.md) to learn about: Exact commands for Python harness checks, Go vet/race tests, bundle installs and native worker acceptance. #testing #ci #commands

## Components (what exists)
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) to learn about: Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks. #ci #docker #release
- Open [**Scenario fixture harness layout**](map-scenario-fixture-harness.md) to learn about: JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness. #testing #fixtures #git
- Open [**Test levels and acceptance suites**](map-test-levels-and-acceptance-suites.md) to learn about: Four test levels from repo quality to real tools in images, plus the worker, Docker and launcher acceptance modules. #testing #docker #acceptance

## By topic

### #testing
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**Run the Python harness and Go suites from the repo root**](practice-run-python-and-go-test-suites.md) — Exact commands for Python harness checks, Go vet/race tests, bundle installs and native worker acceptance.
- Open [**Test levels and acceptance suites**](map-test-levels-and-acceptance-suites.md) — Four test levels from repo quality to real tools in images, plus the worker, Docker and launcher acceptance modules.
### #ci
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.
- Open [**Resolve the image before the container exists, never from inside it**](../runtime/practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #docker
- Open [**Build all PHP images before running Docker acceptance suites**](practice-run-docker-integration-suites.md) — Run docker buildx bake, then docker_worker per runtime and launcher_cli; rebuild the image together with the CLI.
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.
- Open [**Resolve the image before the container exists, never from inside it**](../runtime/practice-select-image-before-container-start.md) — PHP runtime or toolset changes need a different container, so image selection happens outside it (launcher, resolve step, or pinned image).
### #fixtures
- Open [**Cover every new checker with fixtures and failure tests**](practice-checker-test-requirements.md) — Each checker needs clean and failing fixtures, setting-driven cases, failure injection, and real-tool integration.
- Open [**Scenario fixture harness layout**](map-scenario-fixture-harness.md) — JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness.
### #acceptance
- Open [**Test levels and acceptance suites**](map-test-levels-and-acceptance-suites.md) — Four test levels from repo quality to real tools in images, plus the worker, Docker and launcher acceptance modules.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](../configuration/practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](../git-scope/practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](../checkers/practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
### #commands
- Open [**Run the Python harness and Go suites from the repo root**](practice-run-python-and-go-test-suites.md) — Exact commands for Python harness checks, Go vet/race tests, bundle installs and native worker acceptance.
### #git
- Open [**Compute scope once in Git and filter findings by kind**](../git-scope/practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
- Open [**Pre-commit hook analyzes staged files against working-tree context**](../git-scope/practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
- Open [**Scenario fixture harness layout**](map-scenario-fixture-harness.md) — JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness.
### #release
- Open [**CI workflow triggers and jobs**](map-ci-workflow-triggers-and-jobs.md) — Checks validates branches and PRs; Build runs manually or on v* tags with CLI, images or all targets after the same commit passes checks.