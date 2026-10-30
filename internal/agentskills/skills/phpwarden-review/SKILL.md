---
name: phpwarden-review
description: Review scoped code using PHPWarden findings and an evidence-based agent checklist, with project criteria and version-relevant standards. Use for a requested code quality review, validation of edits or prioritization of fixes.
---

# PHPWarden review

Read repository instructions and establish the requested scope, project configuration and available evidence. For a check now, run the project's configured PHPWarden executable with `check --format json` and [the appropriate selection](../phpwarden/references/scope.md). Use known task files for validation after edits. For an explicitly requested diff without further details, use project defaults; do not infer that every unrelated working-tree change belongs to the task. Keep configured checks and policy; a category is a focus, not permission to disable other checks. A user-requested `--only` selection stays explicit in the report.

When asked to review supplied results, reuse them without rerunning analysis unless the task needs a current check. Apply [report identity and freshness rules](../phpwarden-report/references/evidence.md): saved JSON alone does not establish analyzed HEAD/time. Preserve native checker/rule IDs, severity, locations, exit status and completeness, including advisory findings and failed/skipped checks. Read relevant source before asserting impact. Report mismatched or unavailable source as a verification limit.

For an agent review, use [checklist selection and project overrides](references/checklist.md). Read only applicable sections of the bundled checklist or the project's custom questions; an explicitly static-only request needs no manual checklist audit. For language/framework claims follow [standards and version evidence](references/standards.md). Context7 is optional; use official primary documentation or supplied project resources when appropriate. Keep queries free of private source and credentials. Record unavailable/version-mismatched standards rather than inventing compliance.

For each selected question record checked/no issue found, finding, not applicable or not verified, with source evidence and applicable criteria. Separate native findings from agent observations and distinguish confirmed defects from hypotheses. The agent checklist does not change CLI totals, verdicts or exit codes and cannot establish full application security. Follow relevant call paths for access/data flow; check dynamic framework use before recommending deletion of apparently unused code.

Use [architecture](../phpwarden-architecture/SKILL.md), [upgrade](../phpwarden-upgrade/SKILL.md), [tests](../phpwarden-tests/SKILL.md) or [performance](../phpwarden-performance/SKILL.md) only for the relevant requested subtask or existing evidence. Do not start mutation, coverage, profiling or upgrade collectors to fill checklist sections. For a failed setup, use [doctor](../phpwarden-doctor/SKILL.md); preserve incomplete analysis rather than hiding failures or weakening checks/baselines. User-authorized exceptions remain visible context.

Return a concise scoped review with native results, agent checklist observations, standards used, proposed fixes and gaps. Follow [report](../phpwarden-report/SKILL.md) when a Markdown artifact is requested, passing these observations with their own provenance. Apply fixes only within the user's request and recheck their relevant scope. Do not install hooks, rewrite project instructions or create a custom checklist merely to perform the review.
