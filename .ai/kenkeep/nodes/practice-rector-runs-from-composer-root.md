---
type: practice
title: 'Rector runs from composer_root, not the repo root'
description: >-
  Rector resolves installed packages from its working directory, so monorepo
  runs must start in composer_root.
tags:
  - rector
  - monorepo
  - composer-root
kk_schema_version: 3
kk_id: practice-rector-runs-from-composer-root
kk_derived_from:
  - docs/BUGFIXES-2026-10-24.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
Rector's native installed-package resolution uses the process working directory. The adapter therefore executes it from composer_root; running from the repository root in a monorepo cannot find the project's packages.

Rector's JSON output lists unchanged visited files in changed_files while totals.changed_files counts only actual diffs. The adapter validates totals against file_diffs and checks every visited path instead of requiring equal list lengths.

<!-- kk:citations:start -->
# Citations

[1] [docs/BUGFIXES-2026-10-24.md](../../../docs/BUGFIXES-2026-10-24.md)
<!-- kk:citations:end -->
