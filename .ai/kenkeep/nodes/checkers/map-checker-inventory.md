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
  - '01a1693d-cb80-7003-ae47-1468fd6435b0:map:1'
  - '601a78f3-e255-490c-8737-633bc77202bd-1:user-resolution:map'
kk_relates_to:
  - practice-separate-failure-policy-from-findings
  - practice-keep-native-tool-options-in-native-configs
  - practice-analyze-with-project-dependencies
kk_depends_on: []
kk_confidence: high
---
Checker IDs: composer-validate, composer-audit, dead-code, debug, syntax, yaml, whitespace, drupal-config, eslint, eventhorizon, gitleaks, stylelint, jscpd, phpcs, phpmd, phpstan, composer-require-checker, composer-unused, deptrac, rector, semgrep, twig-cs-fixer.

Off by default: composer-require-checker and composer-unused. The Drupal profile enables dead-code, Deptrac and Rector by default. composer-audit is the only checker using the network (in a separate container seeing only copies of composer.json and composer.lock) and is optional by default; set `required: true` in CI. jscpd, phpmd and dead-code findings are warnings and do not fail the check unless `fail_on: warning`. eslint, stylelint and deptrac skip without a project config and never invent rules.

PHPStan needs `composer install` and a `drupal/core` matching `composer.lock`; otherwise it is reported unavailable while the other checkers still run. composer-require-checker and composer-unused need `--mode full`.

The Drupal profile enables Semgrep and EventHorizon by default. Semgrep uses bundled security rules without requiring a project rule file. EventHorizon checks Drupal caching and configuration with advisory findings (`fail_on: never`) but remains required, so failure to execute makes analysis incomplete and unsuccessful. Explicit `enabled: false` in a project file overrides these defaults.

Deptrac requires project architecture rules and skips with no_config when none are discovered; an explicitly configured missing file remains an error. Rector uses bundled upstream presets for the detected Drupal version when no project config, explicit target, sets or rules override them. It always runs as dry-run, with advisory findings (fail_on: never) and required execution. The Composer package analyzers require installed dependencies and a supported package layout as well as full mode.

<!-- kk:related:start -->
# Related

- Related: [practice-separate-failure-policy-from-findings](../reporting/practice-separate-failure-policy-from-findings.md)
- Related: [practice-keep-native-tool-options-in-native-configs](../configuration/practice-keep-native-tool-options-in-native-configs.md)
- Related: [practice-analyze-with-project-dependencies](../runtime/practice-analyze-with-project-dependencies.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] [README.md](../../../../README.md)
[2] 01a1693d-cb80-7003-ae47-1468fd6435b0:map:1
[3] 601a78f3-e255-490c-8737-633bc77202bd-1:user-resolution:map
<!-- kk:citations:end -->
