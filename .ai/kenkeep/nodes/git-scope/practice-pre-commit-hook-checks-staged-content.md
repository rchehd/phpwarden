---
type: practice
title: Pre-commit hook checks staged files and never rewrites the tree
description: >-
  Hook checks whole staged files, aborts on mismatch with the working tree, and
  never stashes, fixes or stages.
tags:
  - git-hook
  - safety
  - pre-commit
kk_schema_version: 3
kk_id: practice-pre-commit-hook-checks-staged-content
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
`phpwarden init --pre-commit` installs the hook; plain `init` does not. The hook checks whole staged files within configured source roots using the project's enabled checks, without comparing to a remote branch, so it works for a first commit and honors Git's temporary index for `git commit -a`. Blocking findings or a required runner/checker failure abort the commit.

Selected files must match their staged versions; if a file has unstaged edits or was deleted only in the working tree, the hook stops with resolution instructions. It never stashes, fixes, stages or rewrites project files, and an index change during checking invalidates the result.

Install resolves Git's effective hooks directory (project-local `core.hooksPath`, linked-worktree common dir), preserves a different existing hook, and records the absolute path of the CLI, so keep the executable there or reinstall. For hook managers, call `phpwarden hook pre-commit`.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
<!-- kk:citations:end -->
