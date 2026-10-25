# PHPWarden's Deptrac adapter

The bundled adapter currently uses Deptrac 4.4.0. Inspect the installed PHPWarden help and project configuration before relying on newer native syntax.

Discovery checks `deptrac.yaml`, `deptrac.yml`, `deptrac.yaml.dist` and `deptrac.yml.dist`, first at the project root, then at the Composer root. `tools.deptrac.config` overrides discovery with a project-relative YAML path. Keep this selection when editing existing rules.

Use the native `deptrac` key for `paths`, `layers` and their collectors, `ruleset`, `exclude_files`, `skip_violations` and analyzer settings. Local YAML imports and baseline files are supported. Native graph `paths` are directories relative to the main config; imports are relative to the importing file. Composer collector `composerPath` and `composerLockPath` are relative to the Composer working directory. Do not point paths outside the project or through symlinks.

Deptrac scans `.php` files. Drupal `.module`, `.inc`, YAML service references, hooks and other dynamic relationships do not become a complete architectural graph automatically. State those limits. Keep vendor/core out of graph traversal where appropriate while still accounting for the external classes referenced by the project's code.

PHPWarden `scope.include` and checker `include`/`exclude` control selected findings; they do not replace the native graph's `paths` or remove context dependencies. Ensure the validation command actually selects the intended source roots; pass project-relative paths when necessary. A config-only diff may be skipped as `no_files`; use full mode after rule changes.

`report_uncovered` exposes uncovered dependencies as warnings; `fail_on_uncovered` treats them as errors. Use them deliberately to identify coverage gaps. `report_skipped` exposes accepted native exceptions. Don't enable strict uncovered enforcement without considering external/framework dependencies.

Explicitly missing/invalid configs, parser failures, timeouts and incomplete graphs remain execution failures regardless of `fail_on`. Findings and execution completeness are separate. When validation fails, retain the native error and propose a relevant configuration or environment correction.
