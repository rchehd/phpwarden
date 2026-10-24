---
type: practice
title: 'Run image tools against the project''s own vendor, never install dependencies'
description: >-
  Tools come from the image, but analyzers read the project's core, contrib and
  autoload; missing deps are reported, not auto-installed.
tags:
  - dependencies
  - phpstan
  - docker
kk_schema_version: 3
kk_id: practice-analyze-with-project-dependencies
kk_derived_from:
  - docs/CONCEPT.md
kk_relates_to: []
kk_depends_on: []
kk_confidence: high
---
All tools run from the PHPWarden image, kept separate from the project's `vendor`. Analyzers such as PHPStan still read the project's own core, contrib, libraries, patches and Composer autoload, because they need the real parent classes, interfaces and APIs of that project. The image's profile supplies compatible tools and rules but does not replace the analyzed site's exact dependency set.

The tool uses a project's dependencies as-is; installing missing project dependencies automatically is out of scope. If a checker cannot run, it must say so, and an incomplete check must never look like a success.

Extra rulesets add configuration only: adding a path to a NEON/XML file does not install a missing rule package, whose code must come from the image or the project's autoload.

<!-- kk:citations:start -->
# Citations

[1] [docs/CONCEPT.md](../../../docs/CONCEPT.md)
<!-- kk:citations:end -->
