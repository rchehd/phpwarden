---
name: phpwarden-upgrade
description: Plan PHP, framework and Composer dependency upgrades using PHPWarden update reports, compatibility constraints and Rector dry runs. Use for a requested update assessment or migration plan.
---

# PHPWarden upgrade

Run the project's configured executable with `updates --format json`. Use the full report even when focusing on security; account for incomplete/stale feeds. For actual application PHP patch information, use a verified runtime version or the explicit `--runtime-version X.Y.Z` option. Composer platform settings and the analysis image are not proof of the deployed runtime.

Prioritize known security advisories and unsupported runtimes/releases, then ordinary patch/minor updates and major migrations. Keep advisory links, affected version ranges and fixed-version evidence. A newer package or security-tagged PHP release alone does not prove the current installation is vulnerable; distributor backports may matter.

Read the `projects` release-status section alongside Composer packages. In the Drupal profile it covers locked packages and packaged code outside Composer. Prioritize upstream revoked, insecure and unsupported status; preserve exact release evidence and its source timestamp. An absent exact tag is unknown, not proof of revocation. Security-policy coverage alone is not a vulnerability, and on-disk metadata does not establish enabled modules or deployed versions. Explain custom/unversioned code and stale or missing Drupal feeds as coverage limits. Unmanaged extensions have no known direct Composer relationship and are excluded by `--direct`.

Inspect Composer requirements, lock, stability, repositories, patches and installed metadata. A version returned by `outdated` is a candidate, not proof the entire dependency set can be installed. Investigate blockers with Composer's `prohibits`/`why-not` in the project's environment when authorized. Explain missing private/path-repository coverage rather than silently dropping it.

For a PHP/framework migration, inspect the current/target versions and the installed toolset's upstream Rector sets. Create or refine native Rector config when the task calls for it, preserving existing rules and skips. Keep code modernization separate from package resolution. Run PHPWarden Rector in dry-run/full mode and list manual changes and validation steps; don't invent sets that the installed bundle lacks.

Produce an ordered upgrade plan with prerequisites, compatible candidates, blockers, code/config migration work and checks. Package installation, runtime/DB changes, Rector application, commits and publishing are separate actions unless already included in the user's authorization. Never run an upgrade merely to discover which updates exist or weaken checks to claim migration success.
