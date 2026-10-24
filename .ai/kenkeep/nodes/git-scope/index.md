# kenkeep Index: git-scope

↑ Parent: [kenkeep](../index.md)

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

> This index only orients you; leaves hold the durable guidance. Open at least one relevant leaf before acting.

## Subfolders
_None._

## Conventions (how we build)
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) to learn about: Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind. #scope #git #findings
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) to learn about: Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines. #diff #scope #checkers
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) to learn about: The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export. #pre-commit #hook #git #scope
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) to learn about: Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages. #git-hook #safety #pre-commit

## Components (what exists)
_None yet._

## By topic

### #scope
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](../checkers/practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
### #git
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
- Open [**Scenario fixture harness layout**](../testing-ci/map-scenario-fixture-harness.md) — JSON-described Git scenarios under tests/fixtures/scenarios, built into real temp Git repos by a Python harness.
### #pre-commit
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.
### #checkers
- Open [**Expose only common options; leave detailed rules in native tool configs**](../configuration/practice-keep-native-tool-options-in-native-configs.md) — PHPWarden fields cover typical needs; special rules stay in the tool's own NEON/XML/PHP/JS config.
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
- Open [**Exakat CE is deliberately not integrated into the image or default pipeline**](../checkers/practice-exakat-ce-deliberately-not-integrated.md) — Exakat CE needs a Gremlin/Java graph runtime, has AGPL obligations and unconfirmed PHP 8.1-8.4 support; revisit only with concrete need.
### #diff
- Open [**Decide per checker whether diff mode reports the whole file or changed lines**](practice-diff-mode-scope-per-checker.md) — Syntax-like checkers report any line of a selected file; style and cross-file checkers filter to changed lines.
### #findings
- Open [**Compute scope once in Git and filter findings by kind**](practice-scope-computation-and-reporting-rules.md) — Diff scope comes from merge-base vs the final working tree, shared by all checkers; findings filter by kind.
### #git-hook
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.
### #hook
- Open [**Pre-commit hook analyzes staged files against working-tree context**](practice-pre-commit-hook-analysis-reads-working-tree-context.md) — The hook selects staged paths but tool configs, dependencies and cross-file context come from the working tree, not an index export.
### #safety
- Open [**Keep project mounts read-only and apply fixes on the host**](../runtime/practice-mount-read-only-apply-fixes-on-host.md) — Checkers never write to the project; formatters emit edits that the launcher hash-validates and applies on the host.
- Open [**Pre-commit hook checks staged files and never rewrites the tree**](practice-pre-commit-hook-checks-staged-content.md) — Hook checks whole staged files, aborts on mismatch with the working tree, and never stashes, fixes or stages.