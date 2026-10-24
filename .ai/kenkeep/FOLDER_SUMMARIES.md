---
schema_version: 1
summaries:
  architecture: >-
    core and profile boundaries, launcher and worker responsibilities; read when
    changing component ownership or framework support
  checkers: >-
    analyzer defaults, bundles, licensing and adapter invariants; read when
    adding tools or changing checker integration
  configuration: >-
    project settings, initialization, native configs and precedence; read when
    adding options or changing config discovery and validation
  git-scope: >-
    Git scope, diff filtering and staged-file hooks; read when changing file
    selection or pre-commit behavior
  reporting: >-
    CLI commands, progress, text and JSON reports, exit codes and failure
    policy; read when changing output or interpreting check outcomes
  runtime: >-
    image selection, project dependencies, isolation, fixes, caches and
    concurrency; read when changing execution or container compatibility
  testing-ci: >-
    CI workflows, fixture harnesses and native and Docker acceptance suites;
    read when changing build gates or validating checker changes
---
# kenkeep Folder Summaries

- `architecture`: core and profile boundaries, launcher and worker responsibilities; read when changing component ownership or framework support
- `checkers`: analyzer defaults, bundles, licensing and adapter invariants; read when adding tools or changing checker integration
- `configuration`: project settings, initialization, native configs and precedence; read when adding options or changing config discovery and validation
- `git-scope`: Git scope, diff filtering and staged-file hooks; read when changing file selection or pre-commit behavior
- `reporting`: CLI commands, progress, text and JSON reports, exit codes and failure policy; read when changing output or interpreting check outcomes
- `runtime`: image selection, project dependencies, isolation, fixes, caches and concurrency; read when changing execution or container compatibility
- `testing-ci`: CI workflows, fixture harnesses and native and Docker acceptance suites; read when changing build gates or validating checker changes
