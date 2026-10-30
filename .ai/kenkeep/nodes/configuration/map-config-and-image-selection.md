---
type: map
title: 'php.runtime detection, image naming and monorepo composer_root'
description: >-
  Init detects php.runtime, which selects the per-PHP-version image;
  composer_root locates Drupal inside a monorepo.
tags:
  - config
  - docker
  - php-version
kk_schema_version: 3
kk_id: map-config-and-image-selection
kk_derived_from:
  - README.md
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:1'
kk_relates_to:
  - map-project-config-v1-schema
  - map-profile-version-image-model
  - practice-analyze-with-project-dependencies
kk_depends_on: []
kk_confidence: high
---
`init` writes `.phpwarden.yaml` at the Git repository root and never overwrites an existing one. The schema is `schemas/config-v1.schema.json`; `examples/drupal.phpwarden.yaml` shows the common settings.

`php.runtime` is the project PHP runtime and selects a compatible image. Source/development CLI uses a preloaded image such as `phpwarden:static-php8.3-dev`; a standalone build with a published manifest selects the pinned registry digest and can download it if missing. Init detects it from `.ddev/config.yaml`, then `composer.json` `config.platform.php`, then the lowest supported version allowed by `composer.lock`. `php.target` is the compatibility range the code must support (8.1 up to the runtime); init defaults it from Composer platform PHP, otherwise from the runtime. An explicit --php-target sets the requested target. Supported PHP versions are 8.1-8.4, with a compatible worker per runtime; the CLI rejects a mismatched runtime. Tool versions and optional capabilities can vary by runtime, as with Composer Require Checker and Infection; use the selected manifest and image labels rather than assuming identical bundles.

In a monorepo `composer_root` records the Drupal project subdirectory; `composer.json`, `composer.lock`, `vendor` and the web root are read from there while diffs and reported paths cover the whole repository. Checker configs in the repository are preferred over those in `composer_root`. Changes are compared against `origin/HEAD` unless `scope.base` is set; PHPWarden never fetches.

<!-- kk:related:start -->
# Related

- Related: [map-project-config-v1-schema](map-project-config-v1-schema.md)
- Related: [map-profile-version-image-model](../runtime/map-profile-version-image-model.md)
- Related: [practice-analyze-with-project-dependencies](../runtime/practice-analyze-with-project-dependencies.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
[2] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:1
<!-- kk:citations:end -->
