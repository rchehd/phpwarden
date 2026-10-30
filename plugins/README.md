# PHPWarden agent plugin packages

New development builds contain eight shared skills for Codex, Cursor and
Claude Code. They do not yet register completion hooks or performance MCP, bundle
the CLI, enforce CLI compatibility or publish to a marketplace. Live client
installation/discovery/update/uninstall acceptance remains pending.

## Agent workflow

Keep client catalogs in the PHPWarden repository, with shared skill sources and
versions tied to the CLI release. Catalog generation and installation recipes
remain pending; a separate marketplace repository is unnecessary for this single
product.

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
| `SHA256SUMS` | SHA-256 of all three archives |

Extract each archive into its own new directory. The archive has no enclosing
directory: the extracted directory is the plugin root. Every archive contains a
README and `phpwarden-plugin.json` recording the client, version, commit,
recommended CLI version, skill inventory and currently unavailable integrations.
The CLI version is a recommendation, not a verified compatibility range.

`internal/agentskills/skills` is the only skill source. The Go embed used by
`init --agents` supplies the same files to the packager, including references,
the Markdown template and links between sibling skills. Generated copies are not
committed. Archive ordering, permissions and timestamps are reproducible.

## CI

Use **Actions → Build → Run workflow → target: plugins**. Image publication must
remain unchecked for this target. After Checks succeeds, the job uploads
`phpwarden-agent-plugins-<commit>` with the three archives and checksums.
`all` and version-tag builds also package the plugins. Ordinary branch pushes
and pull requests run the relevant Go tests; they do not start the Build workflow.

These are development Actions artifacts. The current GitHub Release job still
publishes the existing CLI/container distribution assets; attaching plugin assets,
catalog registration and public publication remain later steps. No marketplace
or global client configuration is changed by this job.

## Try and verify

Follow the README in the selected archive. Claude Code can load an extracted
directory for one session with `claude --plugin-dir /absolute/path/to/plugin`;
plugin skill commands are namespaced, such as `/phpwarden:phpwarden-report`.
For Codex and Cursor use their supported local plugin installation flow, verifying
discovered commands in the installed version. Exact client installation recipes
will be recorded after live acceptance. Avoid installing the same skills both
through the plugin and `init --agents` for one client.

Until plugin acceptance is complete, the existing project integration is:

```sh
phpwarden init --agents codex,cursor,claude -n
```

`init --hooks` and `performance connect` remain separate explicit operations.
Plugin removal does not remove independently installed project integrations or
project rules. Packaging tests check each archive's manifest, shared file bytes,
frontmatter, sibling links, identity, checksums, deterministic content and refusal
to overwrite existing output. They do not certify client discovery or hook trust.

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

No license is invented in manifests. The repository's own distribution license
and public release decision remain open; worker dependency licenses are separate
and no worker dependencies are included in these archives.
