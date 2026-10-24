---
type: practice
title: 'Resolve the image before the container exists, never from inside it'
description: >-
  PHP runtime or toolset changes need a different container, so image selection
  happens outside it (launcher, resolve step, or pinned image).
tags:
  - docker
  - ci
  - image
  - runtime
kk_schema_version: 3
kk_id: practice-select-image-before-container-start
kk_derived_from:
  - docs/CONCEPT.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
A running container cannot change its own PHP version via a YAML field. The image must be chosen before the container is created: by the standalone launcher when Docker is available, by a separate resolve step/job or an explicitly pinned image when CI uses it as a job image, and by prepared image references for dev Compose integration.

Do not assume Compose reads `.phpwarden.yaml`. Changing the PHP runtime or toolset means launching another container, whereas ordinary checker rules are passed to the worker without rebuilding.

The worker verifies that its runtime/toolset matches the config and returns an explicit error on mismatch. Normal analysis needs no Docker inside the worker and no host Docker socket access.

<!-- kk:citations:start -->
# Citations

[1] [docs/CONCEPT.md](../../../../docs/CONCEPT.md)
<!-- kk:citations:end -->
