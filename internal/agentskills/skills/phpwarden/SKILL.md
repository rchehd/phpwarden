---
name: phpwarden
description: Use PHPWarden for a scoped code check or project quality task, selecting the appropriate review, setup, architecture, upgrade, test, performance or report workflow. Use when asked to check code with PHPWarden or choose its workflow.
---

# PHPWarden

Establish the requested outcome, repository instructions, project root and configured PHPWarden executable or wrapper. Read the project's `.phpwarden.yaml` when relevant; `config show --format json` can explain effective settings without running analyzers. Do not substitute an executable found in an unrelated checkout. A missing installation/configuration is setup evidence, not permission to install dependencies or initialize the project automatically.

Choose the smallest relevant workflow below and read its linked skill before acting. Follow it in the current task; routing does not require spawning another agent. Carry the user's scope, supplied evidence and execution constraints through the workflow. Do not run every skill or install hooks/MCP just because PHPWarden was invoked.

| Requested outcome | Workflow |
| --- | --- |
| Install or initialize PHPWarden; plugin onboarding; inspect installation/project readiness | [Setup](../phpwarden-setup/SKILL.md). A status request alone makes no installation or project changes. |
| Check or review code; validate authorized edits | [Review](../phpwarden-review/SKILL.md), including native checks and applicable agent checklist questions. An explicitly static-only request needs no extra manual audit. |
| Diagnose setup or a check that could not run | [Doctor](../phpwarden-doctor/SKILL.md). |
| Create or refine architecture boundaries | [Architecture](../phpwarden-architecture/SKILL.md). |
| Assess updates or plan a migration | [Upgrade](../phpwarden-upgrade/SKILL.md). |
| Inspect existing performance captures | [Performance](../phpwarden-performance/SKILL.md). |
| Explicitly connect, reconnect or disconnect performance MCP for a project/client | [Performance setup](../phpwarden-performance/SKILL.md), using its connection reference. |
| Review coverage, CRAP, mutants or improve tests | [Tests](../phpwarden-tests/SKILL.md). |
| Save a Markdown report from supplied results | [Report](../phpwarden-report/SKILL.md); formatting supplied evidence needs no new check. |

For checks read [scope and command selection](references/scope.md). Explicit paths select whole files; commits/diff and full analysis have different coverage. After code edits, prefer known task files over the entire working-tree diff. Keep the enabled applicable analyzers and policy unless the user explicitly requests a checker selection. A review category is an interpretation focus, not a reason to add `--only`.

For explanation or planning, use available evidence without automatically running analyzers. A current check requested by the user requires current evidence; a saved report alone cannot establish freshness. Preserve native exit/completeness information and separate agent observations. Return the chosen scope, important results and remaining limitations. Changes, heavy collectors and external publication stay within the user's task authorization.
