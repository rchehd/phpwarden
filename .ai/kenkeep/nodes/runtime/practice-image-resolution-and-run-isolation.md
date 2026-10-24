---
type: practice
title: Select images by digest or ID and keep runs isolated and fingerprinted
description: >-
  Image selection is offline and exact, runs are read-only and sandboxed, and
  drift during a run makes results stale.
tags:
  - runner
  - docker
  - images
kk_schema_version: 3
kk_id: practice-image-resolution-and-run-isolation
kk_derived_from:
  - docs/CORE-CONTRACTS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
The host launcher reads config and the release manifest bundled with the CLI, resolves facts, and selects the image by digest (local builds by image ID, never a mutable tag) before the run. The worker executes checkers and emits the same report contract; the host validates the report schema, identity and exit, and re-verifies scope and checksums after the container ends. A worker never pulls or runs nested containers and rejects an incompatible protocol, capabilities, runtime or toolset.

Resolution needs exactly one compatible manifest entry for platform, runtime, framework and target range; zero or ambiguous matches are an input error with no fallback. There is no latest or network manifest update during `check`, and config rule changes never change the toolset. Image overrides and arbitrary rule-package installation are not part of v1.

The project mount is read-only for check and writable only for explicit fix/init. Tool packages stay separate from project `vendor`, and temp configs live in unique run directories whose cleanup removes only resources the current run created. Fixes are prepared in memory by the worker and applied on the host after path and before-hash checks, atomically per file with symlinks rejected.

Inputs (files, effective and native configs, dependency metadata, discovery facts) are fingerprinted before and after the run; a change during the run is a stale or error result, never a pass. Native tool caches are not proof of freshness.

<!-- kk:citations:start -->
# Citations

[1] [docs/CORE-CONTRACTS.md](../../../../docs/CORE-CONTRACTS.md)
<!-- kk:citations:end -->
