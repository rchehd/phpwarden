# Project quality review checklist

Select questions relevant to the task; this file is criteria, not completed
evidence. Add project-specific questions, standards links and documented
exceptions under the appropriate section. Keep stable question IDs.
An unchecked box means not reviewed, never passed. Record results separately
with checked/no issue found, finding, not applicable or not verified, evidence
locations and applicable standards. No CLI configuration is changed by this file.

## Security

- [ ] security.access — Do relevant entry points enforce the intended operation,
  ownership/tenant and field/entity access, including failure paths?
- [ ] security.input-output — Follow untrusted input to queries, markup, files,
  processes and remote requests; are the context-specific boundaries enforced?
- [ ] security.state — For state-changing operations, inspect applicable request
  authenticity, session handling, validation and secret/error exposure.

## Code standards

- [ ] standards.project — Does changed code follow the project's selected naming,
  typing, error-handling and documentation rules beyond native style findings?
- [ ] standards.compatibility — Are suggested APIs/syntax compatible with the
  supported PHP/framework versions and documented project exceptions?

## Correctness

- [ ] correctness.behavior — Do conditions, null/empty values and failure paths
  implement the requested behavior without unintended side effects?
- [ ] correctness.state — Are relevant transactions, resource cleanup,
  concurrency/idempotence and lifecycle assumptions supported by evidence?

## Dependencies

- [ ] dependencies.usage — Are added APIs/packages declared and compatible with
  the selected runtime and project dependency policy?
- [ ] dependencies.evidence — Distinguish actual advisory/update evidence from
  guesses; mark missing/stale feeds or deployed-version uncertainty.

## Maintainability

- [ ] maintainability.intent — Are responsibilities and dependencies clear, with
  complexity/duplication assessed against the task's actual behavior?
- [ ] maintainability.entry-points — Before proposing removal, check framework
  discovery, hooks, plugins, reflection, configuration and dynamic callers.

## Framework quality

- [ ] framework.lifecycle — Do relevant service/plugin/form/event patterns match
  the detected framework version and established project architecture?
- [ ] framework.cache — Where output or access decisions are cached, inspect
  variation, invalidation and lifetime metadata through relevant callers.
- [ ] framework.project — For Drupal code, inspect applicable access results,
  rendering/translation and config/schema conventions using version-aware docs;
  replace this question with the actual framework's criteria for other projects.

## Architecture

- [ ] architecture.boundaries — Does the change respect documented ownership,
  layering and dependency direction, including configured Deptrac boundaries?

## Tests

- [ ] tests.behavior — Do existing/requested tests assert the changed behavior
  and meaningful denied/invalid/error cases? Missing execution is not a pass.
- [ ] tests.evidence — If coverage, CRAP or mutation evidence is supplied, check
  its scope/freshness and what it actually measures before drawing conclusions.

## Performance, when relevant

- [ ] performance.work — Are repeated I/O, query volume and cache assumptions
  supported by code or existing measurements? Separate hypotheses from captures.

## Upgrades, when relevant

- [ ] upgrades.target — Are target versions, supported upstream rules, dependency
  blockers and required manual migration steps established from actual evidence?
