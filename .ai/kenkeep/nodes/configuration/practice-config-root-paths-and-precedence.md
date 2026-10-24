---
type: practice
title: 'Resolve project root and paths, then layer config in fixed order'
description: >-
  Fixed root lookup, relative-path bases and override order; no account-wide or
  local config layers in v1.
tags:
  - config
  - paths
  - precedence
kk_schema_version: 3
kk_id: practice-config-root-paths-and-precedence
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Root: `--project` sets the root; `--config` names the file and its parent is the root when `--project` is absent. If both are given, the file must be inside that root. With neither, search for the nearest `.phpwarden.yaml` from the caller cwd up to the Git worktree root inclusive; none found is an input error suggesting init. The root may be a subdirectory of a Git monorepo; scope and JSON paths are relative to it, and repo files outside the root never enter this project's scope.

Config paths are relative to the config root, positional CLI paths to the caller cwd (normalized before any chdir). Escaping the root, unknown paths, and explicitly selecting a fully excluded path are input errors. Symlinks outside the root are neither traversed nor accepted.

Precedence is core defaults, then profile defaults, then project config, then documented CLI overrides. Scalars replace; lists replace except `exclude`, which is additive; `include` replaces the matching default. v1 has no account-wide rules and no `.phpwarden.local.yaml`.

Native tool configs (NEON/XML) are an adapter layer: their options stand unless the user explicitly sets a supported PHPWarden option. A profile fallback such as a PHPStan level never overrides an existing native config, and PHPWarden neither rewrites native configs nor installs rule packages.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
