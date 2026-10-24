---
type: map
title: PHPWarden result cache scope and keys
description: >-
  Opt-in cache of complete file-only check results, keyed on content, config,
  toolset and image digest, 24h TTL.
tags:
  - cache
  - runner
  - ci
kk_schema_version: 3
kk_id: map-result-cache
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Enabled with `runner.result_cache: true`. It applies only to the file-only checkers `debug`, `syntax`, `yaml`, `whitespace` and `drupal-config` (Composer audit may accompany them, its freshness policy is evaluated every run). Any other enabled checker bypasses the cache because project configs, dependencies and executable autoloaders need live analysis.

Keys include canonical project root, file content hashes, raw and effective config, discovered project facts, selected files and line ranges, checker selection, toolset, runtime and the immutable local image digest. Entries expire after 24 hours; incomplete, stale, interrupted or invalid reports are never reused, while complete reports with findings may be. Hits get a new run ID, and JSON exposes `environment.cache` as `hit`, `miss` or `bypass`.

Use `--no-cache` to force live analysis. Files live under `$XDG_CACHE_HOME/phpwarden` or the OS user cache dir with private permissions; an unavailable cache falls back to live analysis, except `--fix` stops explicitly on unsupported locking. Analyzer caches are private to each run, and concurrent runs use separate temp directories.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
<!-- kk:citations:end -->
