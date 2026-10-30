# PHPWarden skills for @@CLIENT@@

Development plugin package @@VERSION@@, source commit @@COMMIT@@.
Contains eight shared PHPWarden skills, their references, the review checklist
starter and the Markdown report template. The skills are identical to those
installed by `phpwarden init --agents`.

This first package contains skills only. CLI binaries, worker images, completion
hooks and performance MCP configuration are not included. Installing the package
does not install PHPWarden, configure a project or change client settings.
Use the CLI from the same version/commit when available; the recorded version is
a recommendation, not a verified runtime compatibility check.

## Try the package

Verify the archive against its accompanying `SHA256SUMS`, then extract it into a
new directory. The extracted directory is the plugin root: its hidden manifest
and `skills/` must remain together. Use your client's supported local plugin
loading/installation flow. Client discovery and live invocation remain to be
verified; this package is not listed in a public marketplace.

For Claude Code, a temporary session can load the extracted directory with
`claude --plugin-dir /absolute/path/to/extracted-plugin`. Plugin skills are
namespaced; for example `/phpwarden:phpwarden-report`.
Check the installed client's command list for the actual discovered names.

For an existing project without plugin loading, use the established CLI route:
`phpwarden init --agents @@CLIENT@@ -n`. Choose either project-installed skills or
the plugin for that client to avoid duplicate skill discovery. Existing custom
skills are never removed by plugin packaging.

## Use the skills

Start with the `phpwarden` routing skill, or invoke a specialist directly:
`phpwarden-architecture`, `phpwarden-review`,
`phpwarden-doctor`, `phpwarden-upgrade`, `phpwarden-performance`,
`phpwarden-tests` and `phpwarden-report`. Ask the agent to use a skill with the
requested project and scope, for example to read `check.json` and save
`docs/quality-report.md`. Checks use the project's configured PHPWarden executable
and `.phpwarden.yaml`; this package introduces no account-wide project rules.

For a project-specific agent review, supply a checklist path through your request
or project instructions, or use the optional project-root `phpwarden-review.md`.
The starter is `skills/phpwarden-review/assets/review-checklist.md`. Installation
does not create that project file. Review uses relevant criteria and available
Context7/official version-aware sources; missing evidence stays not verified.
Agent observations remain separate from native check counts and verdicts.

Install the host CLI separately using the repository's installation instructions.
Its supported worker images are resolved by the CLI. Heavy testing, mutation
analysis and profiling remain separate operations requiring the user's task.

## Current limits and next steps

Completion hooks and project-specific performance MCP integration remain on the
plugin roadmap. Existing `init --hooks` and `performance connect` integrations
continue to work independently. Trust and client support still apply.
Do not enable duplicate hooks or install another MCP entry for the same runtime.

Updating, rolling back and removing the plugin must use the client's supported
plugin management flow. Project config, installed CLI, project-installed skills,
hooks and performance runtimes have separate lifecycles. Live install/update/
uninstall acceptance and public marketplace publication are pending. The project's own
distribution license is not selected yet; no public licensing grant is implied
by these internal development artifacts.

Format references checked 2026-10-30:

- [Codex supported compatibility layout](https://developers.openai.com/plugins/build/plugins)
- [Cursor plugin structure](https://cursor.com/docs/reference/plugins)
- [Claude Code manifest reference](https://code.claude.com/docs/en/plugins/manifest-reference)
