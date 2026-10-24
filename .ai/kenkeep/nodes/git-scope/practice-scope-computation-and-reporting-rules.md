---
type: practice
title: Compute scope once in Git and filter findings by kind
description: >-
  Diff scope comes from merge-base vs the final working tree, shared by all
  checkers; findings filter by kind.
tags:
  - scope
  - git
  - findings
kk_schema_version: 3
kk_id: practice-scope-computation-and-reporting-rules
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Diff base resolves from CLI, then `scope.base`, then local `refs/remotes/origin/HEAD`. If the ref or merge base is unavailable, return an input error with a fetch/explicit-base hint; never guess main/master and never fetch implicitly. Compare merge-base(base, HEAD) against the final working tree (so committed, staged and unstaged changes count) and add non-ignored untracked files. Changes fully reverted in the working tree produce no findings.

Git paths are NUL-delimited. All checkers share one FileSet/LineMap instead of recomputing diffs. Rename detection is off: a rename is delete plus add, and the new path gets all lines. Modified files use zero-context added ranges (inclusive, 1-based) without external diff or textconv; untracked, added and changed-binary files are all-lines. In JSON, `line_map: null` means all lines and `[]` means a selected file with no added lines.

All three modes (`diff`, `full`, `paths`) require a Git worktree. Symlinks, including internal ones, are not traversed, and non-UTF-8 filenames are an explicit input diagnostic. Globs are case-sensitive, `/`-separated, `*` and `?` do not cross `/`, `**` does; extra excludes can never re-enable core-excluded directories (Git metadata, vendor, node_modules, Drupal core/contrib).

Reporting scope: ordinary findings only in selected files and, in diff mode, only changed lines; syntax findings cover the whole selected file; global findings need explicit checker support and are not a way around scope; process/report failures are diagnostics and never line-filtered. An empty diff is a valid complete run with skipped/no_files checkers, but invalid config is still rejected.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
