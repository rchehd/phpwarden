# PHPWarden · {project}

**Result:** {native verdict} · **Analysis:** {complete / incomplete / not run}

{One paragraph describing the measured outcome, its scope and the next action.}

| Evidence | Value |
| --- | --- |
| Selection | {mode, source/project file counts, base and whole-file behavior} |
| Runtime | {profile/framework and supplied PHP runtime/version} |
| Run | {run ID and source JSON link/hash} |
| Analyzed revision/time | {verified companion evidence, otherwise unverified} |
| Freshness | {verified current / historical / unverified and why} |
| Report generated | {report-writing time, distinct from analysis time} |

## Overview

{Full input totals by native severity; execution diagnostics counted separately.}

| Category | Coverage/status | Errors | Warnings | Info | Notes |
| --- | --- | ---: | ---: | ---: | --- |
| {category} | {status with failures/skips preserved} | {count} | {count} | {count} | {reason/policy} |

## Priority findings

### {Native severity} · {Actionable group title}

**Evidence:** {checker / rule} · {full finding count} · {file count}

{Verified relative source links with positions, or escaped unverified locations.}

{Concise native finding message with sensitive literals redacted if needed.}

**Assessment:** {Confirmed impact or explicitly uncertain interpretation.}

**Suggested action:** {Concrete remediation and a proportionate way to verify it.}

{Repeat for relevant groups; state exact omitted totals when limiting details.}

## Additional evidence

{Only requested/supplied updates, test/coverage/CRAP, mutation or performance evidence.
Keep each source's identity, scope, status, completeness, freshness and limitations.}

## Execution and limitations

{Required/optional execution failures, skipped checks, diagnostics, stale or
unverified inputs, redactions and limits of the selected scope.}

## Next steps

{Short prioritized actions, separating code fixes from environment repairs and
manual verification. State whether this report only recommends changes.}
