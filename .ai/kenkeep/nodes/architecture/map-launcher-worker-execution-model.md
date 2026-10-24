---
type: map
title: Launcher and worker execution layers
description: >-
  PHPWarden splits execution into a host-side Go launcher and an in-image Go
  worker that runs the checks.
tags:
  - architecture
  - docker
  - launcher
  - worker
kk_schema_version: 3
kk_id: map-launcher-worker-execution-model
kk_derived_from:
  - docs/CONCEPT.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Two separate layers exist. The launcher runs on the host: it reads the project config and the compatibility manifest, selects the image/digest, and performs the Docker pull/run. The worker runs inside the image and executes the checkers against the mounted project.

Both are written in Go. The launcher ships as a standalone binary, independent of the agent plugins and of any Claude/Codex account. Only Git and a Docker CLI with a local daemon are required on the host; PHP, Composer, Node.js and checkers live in the image. Host-run computes the Git scope on the host; direct worker-run computes it inside the image.

Agent hooks, manual CLI, dev environments and CI are all adapters over this same launcher, and produce the same result format and exit codes.

<!-- kk:citations:start -->
# Citations

[1] [docs/CONCEPT.md](../../../../docs/CONCEPT.md)
<!-- kk:citations:end -->
