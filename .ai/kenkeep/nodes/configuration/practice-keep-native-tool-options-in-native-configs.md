---
type: practice
title: Expose only common options; leave detailed rules in native tool configs
description: >-
  PHPWarden fields cover typical needs; special rules stay in the tool's own
  NEON/XML/PHP/JS config.
tags:
  - settings
  - config
  - checkers
kk_schema_version: 3
kk_id: practice-keep-native-tool-options-in-native-configs
kk_derived_from:
  - docs/TOOLS-SETTINGS.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
Settings are layered in three tiers: general run/profile parameters, a short set of base fields per checker, and the tool's own native config for detailed rules (PHPStan NEON, PHPCS XML, Twig CS Fixer PHP config, ESLint/Stylelint configs).

Do not mirror all PHPStan, PHPCS, ESLint or Stylelint options in `.phpwarden.yaml`. PHPWarden fields cover typical needs and everything else stays in the native config. Do not offer a free-form shell-argument string as the main way to configure a tool.

CLI, plugin forms and UI share one settings model, with one form (or section) per checker.

<!-- kk:citations:start -->
# Citations

[1] [docs/TOOLS-SETTINGS.md](../../../../docs/TOOLS-SETTINGS.md)
<!-- kk:citations:end -->
