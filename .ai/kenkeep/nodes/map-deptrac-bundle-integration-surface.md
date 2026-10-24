---
type: map
title: Deptrac bundle integration surface and supported project config
description: >-
  How tools/deptrac drives Deptrac through its PHP APIs and which project config
  features it accepts or rejects.
tags:
  - deptrac
  - config
  - bundle
kk_schema_version: 3
kk_id: map-deptrac-bundle-integration-surface
kk_derived_from:
  - tools/deptrac/README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: medium
---
The bundle calls Deptrac's `ServiceContainerBuilder` and `DependencyLayersAnalyser` directly, which preserves the PHP memory limit and yields typed violations, uncovered dependencies, baseline entries and warnings/errors. It selects the standard Nikic parser, loads no persistent cache, keeps generated wrappers and temp files in a private directory, and runs against a read-only project mount. The whole configured `.php` graph is analyzed before findings are filtered to the requested scope.

Project config may use native `deptrac` settings and local YAML imports. Unsupported: PHP configs, custom services/parameters/tags, YAML aliases/anchors, optional/glob/remote imports, and the experimental PHPStan parser flag.

<!-- kk:citations:start -->
# Citations

[1] [tools/deptrac/README.md](../../../tools/deptrac/README.md)
<!-- kk:citations:end -->
