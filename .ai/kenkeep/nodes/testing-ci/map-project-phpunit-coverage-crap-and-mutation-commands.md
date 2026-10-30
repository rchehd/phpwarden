---
type: map
title: Project PHPUnit coverage CRAP and mutation commands
description: >-
  Separate unit-test and Infection runners use isolated project copies, native
  configs, fresh coverage and explicit completeness gates.
tags:
  - testing
  - coverage
  - mutation
kk_schema_version: 3
kk_id: map-project-phpunit-coverage-crap-and-mutation-commands
kk_derived_from:
  - '493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:11'
kk_relates_to:
  - map-cli-surface-and-exit-codes
  - map-test-levels-and-acceptance-suites
  - map-ci-workflow-triggers-and-jobs
  - map-agent-plugin-catalogs-and-shared-skill-delivery
kk_depends_on: []
kk_confidence: high
---
`internal/projecttest` implements phpwarden test and mutation as commands separate from static check and completion hooks. Tests use the project installed PHPUnit and native bootstrap/extensions, with Drupal unit-test discovery or explicit paths/suite/filter. The worker runs a private writable copy without original project/Git/application mounts or network; missing dependencies/extensions and all-skipped or malformed suites cannot pass. Artifacts live in a new output directory or user cache and retain structured reports, native JUnit and stdout/stderr.

Optional test --coverage uses pinned PCOV and upstream php-code-coverage for line coverage and class/trait method CRAP. Native source filters are preserved unless explicitly replaced. CRAP and coverage are distinct from mutation scores; optional thresholds gate complete results. Missing reports/driver or unmeasurable configured gates remain incomplete, not green. HTML/XML/Clover/Crap4J artifacts retain source evidence and full method totals.

Mutation uses the worker Infection bundle and installed PHPUnit on supported PHP 8.3/8.4 without substituting another runtime. It runs fresh initial tests and coverage before mutation in the same private copy; it never reuses another run coverage. Source scope defaults full and can use explicit paths or diff/commits, independently of test selection. Native infection config discovery and explicit overrides preserve allowed bootstrap/mutators/ignores; unsupported integrations are rejected.

Complete runs return 0 unless configured test/coverage/MSI gates fail (1). Invalid input is 2, incomplete execution or no assessed mutations is 3, interruption 130. Procedural functions are outside the pinned mutation traversal and CRAP method gate. The phpwarden-tests skill interprets escaped mutants and hotspots without lowering thresholds or treating suspected equivalent mutants as confirmed; it changes/reruns tests only within the user task. Live native agent behavior is separate from runner/fixture acceptance.

<!-- kk:related:start -->
# Related

- Related: [map-cli-surface-and-exit-codes](../reporting/map-cli-surface-and-exit-codes.md)
- Related: [map-test-levels-and-acceptance-suites](map-test-levels-and-acceptance-suites.md)
- Related: [map-ci-workflow-triggers-and-jobs](map-ci-workflow-triggers-and-jobs.md)
- Related: [map-agent-plugin-catalogs-and-shared-skill-delivery](../architecture/map-agent-plugin-catalogs-and-shared-skill-delivery.md)
<!-- kk:related:end -->

<!-- kk:citations:start -->
# Citations

[1] 493ac963-5aa2-4755-ad08-dc6a1dad21d3:map:11
<!-- kk:citations:end -->
