---
schema_version: 3
nodes_hash: 'sha256:9a2bc0e0901aecd958093059ad2f3e04d098e0f10646c75c9fa7f1e7c50626f1'
node_count: 45
---
# kenkeep

> kenkeep navigation: the injected body above is the root index node, the top-level catalog of branches and root-level leaves. Do not expect the whole knowledge base here; descend on demand. Read the root index node, pick one or more branches whose intent and tags match your task (several branches can be relevant), and read those branch `index.md` nodes. Descend further only where the task needs it, opening only the leaves you have confirmed are relevant. Follow each leaf's `relates_to` and `depends_on` cross edges to reach related leaves in other branches. You decide how deep to go per branch.

## Branches
- Load [`architecture/`](nodes/architecture/index.md) for more information on core and profile boundaries, launcher and worker responsibilities; read when changing component ownership or framework support.
- Load [`checkers/`](nodes/checkers/index.md) for more information on analyzer defaults, bundles, licensing and adapter invariants; read when adding tools or changing checker integration.
- Load [`configuration/`](nodes/configuration/index.md) for more information on project settings, initialization, native configs and precedence; read when adding options or changing config discovery and validation.
- Load [`git-scope/`](nodes/git-scope/index.md) for more information on Git scope, diff filtering and staged-file hooks; read when changing file selection or pre-commit behavior.
- Load [`reporting/`](nodes/reporting/index.md) for more information on CLI commands, progress, text and JSON reports, exit codes and failure policy; read when changing output or interpreting check outcomes.
- Load [`runtime/`](nodes/runtime/index.md) for more information on image selection, project dependencies, isolation, fixes, caches and concurrency; read when changing execution or container compatibility.
- Load [`testing-ci/`](nodes/testing-ci/index.md) for more information on CI workflows, fixture harnesses and native and Docker acceptance suites; read when changing build gates or validating checker changes.
