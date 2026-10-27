---
name: phpwarden-review
description: Review PHPWarden findings in project context, optionally focusing on security, code standards, correctness, dependencies, maintainability, Drupal quality, architecture or upgrades. Use for a requested quality review and prioritization of fixes.
---

# PHPWarden review

Run the project's configured PHPWarden executable with `check --format json` and the user's chosen scope. Keep the configured checks and policies; a category is a focus for the review, not permission to disable the other checks. If no scope was requested, use the project defaults. For an architectural rule change, use full mode.

Read the repository instructions, project configuration and relevant source around each finding. Preserve checker/rule IDs, file locations and original findings. Explain impact and prioritize actionable changes. Separate confirmed defects, plausible risks requiring more evidence and manual verification questions. Do not describe a successful static check as proof that the whole application is correct or secure.

For security, trace relevant entry points, user-controlled inputs, cross-file data flows, access checks, validation, output escaping and reachability. For maintainability, inspect duplication, complexity and dead-code candidates against dynamic framework entry points before suggesting deletion. For dependencies, distinguish known advisories from merely available updates; use `phpwarden updates --format json` when update information is relevant.

For an explicitly requested performance review with existing SPX captures, use `phpwarden performance --reports DIR --producer COMMIT --format json`; get the supported producer identity from `performance --help` and confirm the capture's provenance instead of assuming that identity. Map source paths only with the supplied workspace/container roots. Report incomplete captures as unavailable evidence, distinguish self/inclusive time and overall peak memory, and inspect callers and source before proposing fixes. Capture paths and function names can contain application data; review before sharing. Do not install a profiler, record requests or alter runtime containers without authorization. Profiling is separate from static checks and completion hooks.

When execution is incomplete, report the failed/unavailable checks and use `phpwarden doctor --format json` to investigate. Never hide native findings or weaken rules, add ignores/baselines, or disable checks/hooks to obtain a passing report. Existing user-authorized exceptions remain visible context.

Provide a concise review with severity, evidence, relevant files, proposed fixes and validation limits. Apply changes only when the user's request includes fixing them. Recheck after authorized changes. This deep review is an explicit workflow; completion hooks run deterministic checks rather than invoking another full agent review.
