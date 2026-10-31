# Agent integrations

### Agent skills

Install the shared skills for Codex, Cursor and Claude Code together:

```sh
phpwarden init --agents all --no-interaction
```

Choose specific clients with `--agents codex,cursor` or `--agents claude`; `--agents none` skips installation and does not remove existing skills. In interactive init, press Enter at the agent prompt to skip, or enter `all` or a comma-separated client list.

The installer writes standard Agent Skills packages to `.agents/skills/`. [Codex](https://learn.chatgpt.com/docs/build-skills) and [Cursor](https://cursor.com/docs/skills) discover that directory directly. For [Claude Code](https://code.claude.com/docs/en/skills#choose-where-skills-load), it adds relative directory symlinks under `.claude/skills/` pointing to the same packages. Commit the packages and Claude links to share them with the project. Clients that discover the shared directory can see the skills regardless of which client originally requested installation.

Start with `$phpwarden` in Codex or `/phpwarden` in Cursor and Claude Code, for example
`$phpwarden review path/to/file.php`. It chooses the relevant workflow and scope.
You can also invoke a specialist directly using the same convention:

| Skill | Purpose |
| --- | --- |
| `phpwarden` | Select a task-scoped workflow; route checks, setup, architecture, upgrades, tests, performance or reports to the relevant specialist. |
| `phpwarden-setup` | Locate or explicitly install the host CLI, initialize a project when requested, and separate CLI/config/worker readiness. |
| `phpwarden-architecture` | Inspect the project and propose native Deptrac boundaries, then validate the complete graph. |
| `phpwarden-review` | Combine native findings with an applicable agent checklist, project criteria and version-relevant standards. Category focus keeps configured checks intact. |
| `phpwarden-doctor` | Explain environment failures using the native `doctor` report and propose concrete setup repairs. |
| `phpwarden-upgrade` | Plan package/PHP upgrades from `updates`, identify dependency blockers, and use upstream Rector sets in dry-run mode. |
| `phpwarden-performance` | Explicitly connect/disconnect project performance MCP, or review recorded SPX hotspots, callers and bounded graphs while preserving scope and measurement limits. |
| `phpwarden-tests` | Review escaped mutants and CRAP hotspots, propose behavioral assertions, and improve and recheck tests when requested. |
| `phpwarden-report` | Create a Markdown quality report from JSON evidence, with category totals, prioritized findings, source links and analysis limitations. |

For example, ask `$phpwarden-tests review /path/to/mutation/report.json and /path/to/coverage/report.json` in Codex. To apply changes, ask it to improve the relevant tests and rerun them. The skill checks report freshness and completeness, distinguishes test gaps from suspected equivalent mutations, and preserves the configured thresholds. It runs mutation analysis separately from ordinary static checks and completion hooks.

The review skill accepts a checklist explicitly named in the task/project
instructions, or the optional project-root `phpwarden-review.md`. A starter is
bundled at `phpwarden-review/assets/review-checklist.md` inside the installed
skills. Copy it only when setting up your own project criteria; init does not
create or overwrite that project file. No new YAML option is needed.
Context7 can supply standards when available; official documentation or supplied
project resources are alternatives. The skill verifies version applicability,
reports missing evidence and distinguishes agent observations from native
findings. It does not install Context7 or run a full audit for every small change.

For a shareable report, ask `$phpwarden-report use check.json and save docs/quality-report.md`
in Codex, or `/phpwarden-report` with the same request in Cursor/Claude. The agent
uses a bundled template and writes in your requested language. It preserves native
totals, advisory findings, skipped/failed checks and unknown freshness; supplied
JSON alone cannot prove the analyzed HEAD or execution time. Existing optional
update, coverage, mutation and performance evidence can be included without
starting those collectors. The default destination is `phpwarden-report.md` in
the project root; existing files are preserved using an unused timestamped name.

Repeated installation reuses identical files and links. Different existing files, conflicting links and symlinked parent directories stop installation without overwriting them. Review customized skill files before updating them. A write failure can leave completed installation files; init reports the failure and a retry can finish it. Skill installation preserves project configuration, harness settings and hooks.

### Agent project instructions

Install a short project rule alongside skills:

```sh
phpwarden init --agents all --instructions all -n
```

`--instructions codex,cursor` selects clients; `none` skips without removing
existing instructions. Instructions and skills are independent selections, so
`--instructions all` can also be used with an already installed plugin. The rule
asks the agent to use the available `phpwarden` skill and report missing setup
if unavailable. No automatic check or completion hook is registered.

The installer adds one shared managed block to `AGENTS.md`. Claude also gets a
managed `@AGENTS.md` import in `CLAUDE.md`, unless that exact import already
exists. This supports older Claude versions and projects whose Claude settings
prefer CLAUDE.md. It imports the project's existing AGENTS.md rules as well.
Codex/Cursor share AGENTS.md, so any client reading that file can see the rule.
Nested/user/managed instruction precedence still applies; reload the client and
verify discovery. [Cursor instructions](https://cursor.com/docs/rules) and
[Claude imports](https://code.claude.com/docs/en/memory#share-one-file-with-other-coding-tools)
describe the client behavior.

User text outside the managed markers is preserved, including line endings and
permissions. Repeating setup merges the selected clients without duplicating the
block. Modified managed text, ambiguous markers, symlinks and oversized/non-UTF-8
files stop preflight. Put custom rules outside the managed block or in your
project checklist. Installation rechecks files under a project lock and writes
atomically; an I/O failure can leave earlier completed steps and names the error.

Remove only the owned instructions through the host CLI:

```sh
phpwarden instructions remove --client all --project /path/to/project
```

Selective removal retains the shared rule for remaining clients. Removal preserves
skills, hooks, project config, foreign imports and text; empty files created solely
by this installer are removed. A modified managed block is left for manual
reconciliation. Removal with explicit `--project` also works after config deletion.
Instructions guide an agent; CI or explicitly installed hooks remain the separate
choice for an enforced gate.

### Agent plugin development packages

The `phpwarden-setup` skill handles new-machine/project setup and read-only status.
Codex plugin metadata declares it as the onboarding skill; Claude Code and Cursor
can invoke the same skill explicitly. For example, ask "Set up PHPWarden for this
project" or "Show PHPWarden installation and worker status without changing anything".
The plugin includes a small shell bootstrap and the canonical installer, not a CLI
binary. No session-start hook or automatic installation runs merely by enabling it.

Setup prefers a project-managed Composer/npm CLI, then PATH and the user-local
installation. Missing CLI can be installed from an available published release,
an explicitly chosen successful Build CI run, or a supplied verified offline
archive. CI candidates must include a published worker manifest; `target: plugins`
and CLI-only source builds cannot provide a clean-machine automatic worker setup.
CI downloads need an already authenticated `gh`; releases need curl. Setup does
not install these host prerequisites. Existing CLI files and managed packages are
reused, and a version change requires an explicit update action. Project init,
image download, hooks and MCP remain separately scoped operations.

The same nine skills can be packaged for Codex, Cursor and Claude Code through
**Actions → Build → Run workflow → target: plugins**. The development artifact
contains three client ZIPs, a self-contained catalog ZIP and `SHA256SUMS`;
it requires no CLI or worker image build. Repository catalogs for all three
clients point to one shared plugin tree. Version-tag releases attach the same
plugin packages alongside the CLI. These packages contain skills only. Plugin
hooks, performance MCP wiring, live client acceptance and public marketplace
publication remain pending. The existing `init --agents` route
continues to provide project installation. See [plugin packaging](../plugins/README.md)
for local builds, package contents and current installation limits.

### Agent completion hooks

```sh
phpwarden init --agents all --hooks all --no-interaction
```

`--hooks codex,cursor` selects clients; `--hooks none` skips installation without removing existing hooks. The installer merges its command into `.codex/hooks.json`, `.cursor/hooks.json`, and `.claude/settings.json`, preserving other settings and handlers. Invalid/ambiguous JSON or symlinked settings stop preflight. Its managed handler is updated on repeated init, without adding duplicates. Installation can finish partially after an I/O failure; the error names the incomplete step.

The command contains the local executable's absolute path. Run init on each machine to update that path when the CLI moves. In [Codex](https://learn.chatgpt.com/docs/hooks), review and trust the exact hook via `/hooks`; installing files does not grant that trust. Project trust and hook support in the installed client are required. Client discovery and live completion behavior must be verified in your client version.

At completion, the hook runs the ordinary configured `check` scope and failure policy. Required execution failures also request correction. A clean scoped diff skips the worker; deleted/project files remain part of scope handling. Checks serialize per project. Results are checked afresh; the existing result cache is used only when its normal eligibility and freshness checks allow it. No code, baseline, check setting or dependency is changed automatically.

[Claude Code](https://code.claude.com/docs/en/hooks) and Codex receive a blocking Stop decision; [Cursor](https://cursor.com/docs/hooks) receives a follow-up message. Cursor stop hooks request continuation rather than enforce a commit gate. Aborted/error runs and nested PHPWarden executions are ignored. Automatic correction is bounded to two attempts per user turn by default; unresolved results are reported after that limit. A new user turn starts a new budget. Optional check failures remain nonblocking but are reported as incomplete. Cursor prints nonblocking limitations to the hook's stderr. Multi-root Cursor workspaces need a manually selected `--project PATH` in the hook command.

```yaml
agents:
  completion:
    enabled: true
    max_attempts: 2
    timeout_seconds: 300
```

`max_attempts` accepts 1–5; timeout accepts 1–540 seconds. The installed handler allows 600 seconds for checking and cleanup. Set `enabled: false` to suspend installed completion checks. Git pre-commit remains a separate integration.
