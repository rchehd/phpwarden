---
type: map
title: PHPWarden checker inventory and defaults
description: >-
  Checker IDs, which are opt-in, advisory, networked or config-dependent, and
  per-checker prerequisites.
tags:
  - checkers
  - config
  - defaults
kk_schema_version: 3
kk_id: map-checker-inventory
kk_derived_from:
  - README.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Checker IDs: composer-validate, composer-audit, dead-code, debug, syntax, yaml, whitespace, drupal-config, eslint, eventhorizon, gitleaks, stylelint, jscpd, phpcs, phpmd, phpstan, composer-require-checker, composer-unused, deptrac, rector, semgrep, twig-cs-fixer.

Off by default: composer-require-checker, composer-unused, dead-code, deptrac, eventhorizon, rector, semgrep. composer-audit is the only checker using the network (in a separate container seeing only copies of composer.json and composer.lock) and is optional by default; set `required: true` in CI. jscpd, phpmd and dead-code findings are warnings and do not fail the check unless `fail_on: warning`. eslint, stylelint and deptrac skip without a project config and never invent rules.

PHPStan needs `composer install` and a `drupal/core` matching `composer.lock`; otherwise it is reported unavailable while the other checkers still run. composer-require-checker and composer-unused need `--mode full`.

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../README.md)
<!-- kk:citations:end -->
