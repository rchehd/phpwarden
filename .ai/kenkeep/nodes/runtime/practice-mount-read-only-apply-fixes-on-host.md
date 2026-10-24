---
type: practice
title: Keep project mounts read-only and apply fixes on the host
description: >-
  Checkers never write to the project; formatters emit edits that the launcher
  hash-validates and applies on the host.
tags:
  - safety
  - fix
  - docker
kk_schema_version: 3
kk_id: practice-mount-read-only-apply-fixes-on-host
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Checks mount the project read-only. Formatters (PHPCS and Twig CS Fixer, the only checkers that support `--fix`) prepare edits while still read-only; the launcher validates input hashes and applies replacements on the host, preserving permission bits. Requesting any other checker with `--fix` is an input error.

Concurrent changes detected before writing abort the operation. Each file replacement is atomic, but a later write failure can leave earlier replacements applied; the report lists applied files in `fixed_files`. Fixes on the same project wait for one another without creating lock files in the source tree.

Fixes may touch lines outside diff hunks, and the final recheck covers all lines of the original selection and never uses the result cache. Other analyzers (Rector, Semgrep, Gitleaks, EventHorizon) run on dry-run or private copies and must not write into the project.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
<!-- kk:citations:end -->
