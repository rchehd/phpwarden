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
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
`init` writes `.phpwarden.yaml` at the Git repository root and never overwrites an existing one. The schema is `schemas/config-v1.schema.json`; `examples/drupal.phpwarden.yaml` shows the common settings.

`php.runtime` is the PHP version the project runs on and selects the image, e.g. `8.3` maps to `phpwarden:static-php8.3-dev`. Init detects it from `.ddev/config.yaml`, then `composer.json` `config.platform.php`, then the lowest supported version allowed by `composer.lock`. `php.target` is the version the code must support (8.1 up to the runtime). Supported PHP versions are 8.1-8.4, one image each with identical checker versions; the CLI rejects an image from another version.

In a monorepo `composer_root` records the Drupal project subdirectory; `composer.json`, `composer.lock`, `vendor` and the web root are read from there while diffs and reported paths cover the whole repository. Checker configs in the repository are preferred over those in `composer_root`. Changes are compared against `origin/HEAD` unless `scope.base` is set; PHPWarden never fetches.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
<!-- kk:citations:end -->
