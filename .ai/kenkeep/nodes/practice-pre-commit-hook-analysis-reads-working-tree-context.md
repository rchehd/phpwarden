---
type: practice
title: Pre-commit hook analyzes staged files against working-tree context
description: >-
  The hook selects staged paths but tool configs, dependencies and cross-file
  context come from the working tree, not an index export.
tags:
  - pre-commit
  - hook
  - git
  - scope
kk_schema_version: 3
kk_id: practice-pre-commit-hook-analysis-reads-working-tree-context
kk_derived_from:
  - docs/PRE-COMMIT-2026-10-24.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
The pre-commit hook is not an isolated export of the Git index. Only the selected file set comes from staged paths; tool configs, dependencies and cross-file context are those of the working tree. A staged regular file that the worker inventory cannot represent (for example an ignored file force-added only to an alternate index) fails explicitly instead of being silently accepted.

Empty, deletion-only or inapplicable selections skip the worker. A staged-added file deleted only from the working tree is rejected, not skipped. The hook helper emits text only and accepts just the project, color and all display options.

<!-- kk:citations:start -->
# Citations

[1] [docs/PRE-COMMIT-2026-10-24.md](../../../docs/PRE-COMMIT-2026-10-24.md)
<!-- kk:citations:end -->
