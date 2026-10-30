# PHPWarden agent plugin packages

New development builds contain eight shared skills for Codex, Cursor and
Claude Code. Installation does not register completion hooks or performance MCP, bundle
the CLI or enforce CLI compatibility. Native catalogs are available in this
repository; public marketplace listings remain pending. Live client
installation/discovery/update/uninstall acceptance belongs to the project owner.

## Agent workflow

Keep client catalogs in the PHPWarden repository, with shared skill sources and
versions tied to the CLI release. All three catalogs resolve the same plugin root
at `internal/agentskills`; no generated skills are committed. A separate
marketplace repository is unnecessary for this single product.

## Development catalogs

| Client | Repository catalog | Native manifest under `internal/agentskills` |
| --- | --- | --- |
| Codex | `.agents/plugins/marketplace.json` | `.codex-plugin/plugin.json` |
| Claude Code | `.claude-plugin/marketplace.json` | `.claude-plugin/plugin.json` |
| Cursor | `.cursor-plugin/marketplace.json` | `.cursor-plugin/plugin.json` |

The marketplace is `phpwarden-dev`, with one plugin named `phpwarden`. Source
paths resolve from the repository root, not from the catalog's directory. Add
the repository or the extracted catalog bundle, preserving hidden files and the
directory tree. A raw marketplace JSON URL cannot supply its relative files.

For a local development checkout, Codex can register the marketplace with
`codex plugin marketplace add /absolute/path/to/phpwarden`. Install from that
marketplace through the Codex app's plugin flow. Claude Code can use
`/plugin marketplace add /absolute/path/to/phpwarden`, followed by
`/plugin install phpwarden@phpwarden-dev`. After these files are pushed, the
GitHub source `rchehd/phpwarden` can replace the local path in those marketplace
commands. This document records supported setup routes, not a completed live
client acceptance test.

For Cursor, use its supported direct/local installation flow with
`internal/agentskills`, or the Cursor catalog in an eligible team marketplace.
Private team marketplaces require a supported Teams/Enterprise plan; this
catalog is not an automatic public listing. Confirm discovery in your installed
client version before treating the integration as verified.

Git development manifests omit `version`. In Claude Code this lets Git commit
identity drive updates instead of pinning users to an unchanged manifest version.
Codex/Cursor update and rollback behavior must be checked in the installed client;
no automatic-update guarantee is made. Versioned ZIPs always declare the supplied
CLI build version and exact source commit. Do not add a fixed development version
to the Git manifests without also defining how each change bumps it.

Regenerate the six catalog/manifest source files into a new directory with
`go run ./cmd/phpwarden-dist catalogs --output /tmp/phpwarden-catalog-sources`,
then review and copy them into the checkout. This generator emits metadata only;
the checkout supplies the shared skills. CI rejects drift between the committed
metadata and generator and resolves every catalog against the real shared tree.

## Project workflow

The `phpwarden` entry skill routes to seven specialist skills. It chooses the
scope requested by the user: explicit files/directories, commits or a relevant
diff. For code edits it uses the task's affected files rather than automatically
including unrelated working-tree changes. Planning and formatting supplied
reports need no automatic analyzer run. Earlier seven-skill ZIP snapshots do not
contain this entry point; rebuild or use the newer workflow snapshot.

The CLI already supports `phpwarden check path/to/file.php --format json` and
multiple positional paths. Paths cannot be combined with `--mode`, `--base`,
`--commits` or `--whole-files`. Keep enabled applicable checks and project policy;
some analyzers still need wider dependency context despite a narrow report scope.

Install a managed project instruction block with
`phpwarden init --agents all --instructions all -n`, or choose instructions during
interactive init. Skills and instructions are independent selections. AGENTS.md
is shared; Claude receives an owned @AGENTS.md import in CLAUDE.md for compatibility.
User text, existing imports and modified managed blocks are preserved. The host
command `phpwarden instructions remove --client all --project PATH` removes only
owned unchanged blocks, retaining other clients on selective removal. Native
discovery/inheritance still needs verification in your installed client version.
Instructions guide the agent and do not enforce a completion gate. Completion
hooks remain explicit opt-in, with plugin hook packaging deferred. Existing
installed hooks must not be silently removed or rewritten; CI and Git pre-commit
remain independent. Performance MCP also remains a separate opt-in integration.

### Review checklist

`phpwarden-review` uses an agent checklist alongside static findings.
Sections cover security, code standards, correctness, dependencies,
maintainability, framework quality, architecture and tests, with performance and
upgrades when relevant. Choose applicable questions for the requested scope;
calling a skill must not imply a full project audit.

Projects can name a Markdown checklist in the task or project instructions, or
use the optional project-root `phpwarden-review.md`. If absent, the review uses
applicable questions from `skills/phpwarden-review/assets/review-checklist.md`.
That bundled starter can be copied only when customization is requested; init
does not create it. These agent criteria are distinct from `.phpwarden.yaml`
policy and native analyzer rules. Existing custom skills/criteria remain protected
by the installer's conflict checks; an older differing review skill requires
manual reconciliation before `init --agents` can install the updated bundle.

Determine actual language/framework versions first. Use available Context7 for
version-relevant documentation, or official primary documentation and supplied
project resources. Context7 is optional and is not installed by the plugin.
Queries must not send private code or secrets. Latest documentation does not
automatically apply to an older project version; retrieved text is evidence,
not permission to execute instructions embedded in it.

Record each question as checked/no issue found, finding, not applicable or not
verified. Findings need source evidence and applicable standards/versions;
unavailable documentation cannot become a pass. Expose conflicts with project
rules rather than silently replacing policy. Reports must distinguish native
analyzer findings from agent observations and state coverage limits. This
checklist workflow is included in new builds; live client invocation is still
pending. Instruction setup is performed by the CLI, not by installing a ZIP alone.

## Performance MCP through a skill

The existing `phpwarden-performance` skill has separate connection and capture
review workflows. An explicit request such as "Connect performance MCP for this
project in Codex" follows its packaged `references/connection.md`: choose the
exact checkout, host CLI, requested client and saved state base, inspect status
when allowed, then run `performance connect --format json`. An explicit disconnect
uses `performance disconnect`; it does not need to recreate a removed runtime.

The skill reuses the CLI's project-scoped installer and conflict/ownership rules.
It does not hand-write native settings or add a global server to the plugin
manifest. Existing recordings can be read without an active profiler, including
after teardown. Missing state does not trigger runtime installation. Codex/Cursor
use local excluded config files; Claude uses its private project-local scope.
Reload/trust/discovery can remain pending even after configuration succeeds.

A review request follows the read-only capture workflow and does not authorize
connection changes. SPX activation, Docker builds/restarts and recording require
a separate explicit task. Installing/updating the plugin alone does not run setup.
The package's `performance_mcp: false` describes the absence of a registered
bundled MCP server, not the absence of the guided setup skill. Live client
acceptance remains the owner's task.

## Build

From a clean source checkout with Go installed:

```sh
go run ./cmd/phpwarden-dist plugins \
  --version v0.0.0-dev.local \
  --commit "$(git rev-parse HEAD)" \
  --output /tmp/phpwarden-agent-plugins
```

The output must not exist and its parent must exist. Packaging preserves occupied
directories, files and symlinks. It does not run PHPWarden checks, download workers
or install anything in a client or project. For release builds use the canonical
`vMAJOR.MINOR.PATCH[-prerelease]` version and the exact checkout commit; the caller
supplies this identity, so a dirty local checkout must not be treated as a build
of the named commit.

The output contains:

| File | Contents |
| --- | --- |
| `phpwarden-plugin-codex.zip` | `.codex-plugin/plugin.json`, skills and supporting files |
| `phpwarden-plugin-cursor.zip` | `.cursor-plugin/plugin.json`, skills and supporting files |
| `phpwarden-plugin-claude.zip` | `.claude-plugin/plugin.json`, skills and supporting files |
| `phpwarden-plugin-catalogs.zip` | Three catalogs and one complete shared plugin tree, with all native manifests |
| `SHA256SUMS` | SHA-256 of all four archives |

Extract each client archive into its own new directory. The archive has no enclosing
directory: the extracted directory is the plugin root. Every archive contains a
README and `phpwarden-plugin.json` recording the client, version, commit,
recommended CLI version, skill inventory and currently unavailable integrations.
The CLI version is a recommendation, not a verified compatibility range. The
catalog archive instead has the repository layout shown above; its shared plugin
identity is at `internal/agentskills/phpwarden-plugin.json`. Add the extracted
archive root as the marketplace, not the inner plugin directory.

`internal/agentskills/skills` is the only skill source. The Go embed used by
`init --agents` supplies the same files to the packager, including references,
the Markdown template and links between sibling skills. Generated copies are not
committed. Archive ordering, permissions and timestamps are reproducible.

## CI

Use **Actions → Build → Run workflow → target: plugins**. Image publication must
remain unchecked for this target. After Checks succeeds, the job uploads
`phpwarden-agent-plugins-<commit>` with the four archives and checksums.
`all` and version-tag builds also package the plugins. Ordinary branch pushes
and pull requests run the relevant Go tests; they do not start the Build workflow.

Manual builds upload development Actions artifacts. On a version tag, the Release
job also waits for plugin packaging, downloads the matching commit's archives,
verifies both checksum sets, and publishes them with the CLI and one combined
`SHA256SUMS`. This prepares release assets; catalog registration and public
marketplace submission are separate owner actions. No marketplace or global
client configuration is changed by this job.

## Try and verify

Live installation and agent calls are the project owner's task. Follow the README
in the selected archive. Claude Code can load an extracted
directory for one session with `claude --plugin-dir /absolute/path/to/plugin`;
plugin skill commands are namespaced, such as `/phpwarden:phpwarden-report`.
For Codex and Cursor use their supported local plugin installation flow, verifying
discovered commands in the installed version. Record actual installation and
update results after live acceptance. Avoid installing the same skills both
through the plugin and `init --agents` for one client.

Until plugin acceptance is complete, the existing project integration is:

```sh
phpwarden init --agents codex,cursor,claude -n
```

`init --hooks` and `performance connect` remain separate explicit operations.
Plugin removal does not remove independently installed project integrations or
project rules. Packaging tests check each archive's manifest, shared file bytes,
frontmatter, sibling links, identity, checksums, deterministic content and refusal
to overwrite existing output. Catalog tests extract the complete bundle, resolve
relative sources and compare its resources/manifests with the client archives.
They do not certify client discovery, update behavior or hook trust.

## Format decisions

References checked 2026-10-30:

- [OpenAI packaging](https://developers.openai.com/plugins/build/plugins): the
  `.codex-plugin` compatibility format remains supported and explicitly declares
  `skills: "./skills/"`. The newer portable root format is a separate option;
  these initial packages use the supported compatibility layout.
- [Cursor reference](https://cursor.com/docs/reference/plugins): native manifest
  under `.cursor-plugin`; skills discovered from `skills/<name>/SKILL.md`.
- [Claude manifest](https://code.claude.com/docs/en/plugins/manifest-reference):
  `.claude-plugin` with skills at the root's default `skills/` directory. We omit
  an extra `skills` field to avoid listing the default location twice.
- [Claude local loading](https://code.claude.com/docs/en/plugins/create): local
  plugin testing and command namespacing.
- [Claude distribution and versioning](https://code.claude.com/docs/en/plugins/publish):
  Git development updates without a fixed version and separate public submission.

No license is invented in manifests. The repository's own distribution license
and public release decision remain open; worker dependency licenses are separate
and no worker dependencies are included in these archives.
