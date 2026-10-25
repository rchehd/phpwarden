---
name: phpwarden-architecture
description: Create or refine project-specific Deptrac architecture rules for PHPWarden after inspecting the project's intended boundaries and existing dependencies. Use when setting up architecture checks or changing dependency rules.
---

# PHPWarden architecture

Create useful, project-specific dependency rules and validate them with PHPWarden's Deptrac integration. Preserve the project's intended architecture rather than making every existing dependency legal.

## Discover the project

- Read the repository's agent instructions and architecture documents, `.phpwarden.yaml`, Composer manifests and existing Deptrac configuration, including imports and baselines.
- Use `phpwarden config show --format json` to resolve the profile, Composer root and current settings. Use the project's configured PHPWarden executable; if it is not on PATH, locate it or report the missing installation.
- Inspect namespaces, source roots, module/package boundaries, service definitions, entry points and representative tests. For Drupal, inspect custom modules, `.info.yml` dependencies, service YAML, plugins and procedural entry points. For other profiles, use that framework's actual conventions.
- Separate observed dependencies from intended boundaries. Prefer documented decisions and public APIs; identify uncertain boundaries for discussion when they materially affect the rules.

## Write native rules

Create `deptrac.yaml` at the project or Composer root when no configuration exists. If `tools.deptrac.config` selects another file, use that path. Extend existing rules without replacing unrelated layers, imports, exclusions or accepted exceptions. Explain any proposed relaxation separately; don't add baselines, disable checks or permit every observed edge merely to obtain a passing report.

Choose meaningful layers and collectors based on the inspected project. A small module may not need domain/application/infrastructure layers. Specify concrete native `paths`, `layers` and `ruleset` under the `deptrac` key; do not invent a framework-wide architecture preset. Keep dependency rules at class/package level, not just directory labels. Explain permitted directions and which source files are outside coverage.

Read [the adapter reference](references/deptrac.md) before writing rules, especially for monorepos, imports and collector paths.

## Validate and report

Run from the project root:

```sh
phpwarden check --only deptrac --mode full --no-cache --format json
```

Rule changes can affect unchanged callers, so a diff-only result is insufficient. Inspect both findings and completeness: exit 0 alone does not prove the intended graph was checked. A skipped check, unavailable dependency, parser failure or incomplete graph is not successful validation. If the configured graph has no applicable source files, report that explicitly.

Report the chosen layers, their rationale, allowed dependency directions, remaining violations and uncovered areas, with file references where available. Distinguish forbidden dependencies from configuration or environment failures. Keep useful rules even if existing code violates them; don't refactor application code unless the user's task includes it.

When images or dependencies are missing, explain what prevents validation. Building images, installing project dependencies, changing the runtime, applying fixes, committing and publishing require authorization within the current task; don't perform them merely to finish architecture setup.
