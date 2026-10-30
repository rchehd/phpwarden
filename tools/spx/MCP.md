# Read-only performance MCP server

`phpwarden performance mcp` serves the guarded native SPX reader over stdio.
It uses the official Go MCP SDK pinned in `go.mod`; the raw upstream PHP MCP
server from the compatibility spike is not embedded or launched. The host needs
no PHP, Composer, Docker daemon or network to read captures.

```sh
phpwarden performance mcp --reports /absolute/path/to/reports \
  --producer 62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07 \
  --project /absolute/path/to/project --container-root /var/www/html
```

Use the reports path from `performance status` or from the saved capture
directory. Source mapping is optional and needs both mapping arguments. Use
`--report KEY` at startup to restrict the server to one capture, including in a
directory with more than the default 25-report scan limit. An empty directory
can be selected before capturing; queries then report no available inputs.

The chosen directory and optional source workspace are opened and anchored for
the server's lifetime. A directory replacement produces an incomplete result;
it does not redirect the server into a different directory. Root/file checks,
whole-stream validation, compression/count/resource limits and metadata
redaction are the same as in the CLI reader. stdout contains JSON-RPC only;
startup/session diagnostics use stderr. No server command changes client
configuration, records requests, executes project code or manages containers.

## Connect a client locally

After preparing the runtime with `performance setup`, use the host CLI:

```sh
phpwarden performance connect --project /absolute/path/to/project --client all
phpwarden performance disconnect --project /absolute/path/to/project --client all
```

`--client` accepts `all`, `codex`, `cursor`, `claude` or a comma-separated
selection. Both commands support `--state-dir` and `--format json`. Use the same
exact project and state base as setup. The connection reads the producer, reports
directory and source mapping from that saved setup, including after teardown
when captures remain. It does not build images or start containers.

Codex uses `.codex/config.toml`; Cursor uses `.cursor/mcp.json`. New files get
exact entries in the checkout's local Git exclude. Tracked configs are refused,
and existing untracked configs must already be ignored so connecting does not
hide unrelated user files. Claude uses the selected project's entry in
`~/.claude.json`, or `$CLAUDE_CONFIG_DIR/.claude.json` when configured; it does not
create a shared `.mcp.json` or a server available to every project.

All selected configs are checked before writing. Existing servers and settings
are retained; JSON is reformatted without changing values, while foreign TOML
bytes and comments remain intact. Duplicate keys, invalid scopes, symlinks and
server-name conflicts fail explicitly. Sealed inline TOML server tables may
require manual configuration. Private ownership records contain only managed
entry fingerprints and generated blocks, never copies of foreign settings.
Repeating connect is safe and can update the executable path. An identical
pre-existing server is reused without claiming ownership.

Disconnect removes only unchanged owned entries. Manual changes to those
entries require review and are not overwritten. Other settings, captures and
runtime state are retained; local exclusions remain if the config now contains
additional user settings. Disconnect can run without the original runtime
receipt. If a write fails, earlier settings writes are rolled back when they
have not changed concurrently, and a private pending ownership receipt allows
retrying. Avoid editing client settings concurrently with installation.

Reload the selected clients to discover the tools. Codex additionally requires
trusting the project; connection installation does not change trust settings.
The commands configure the connection, not client approval or enablement UI.

The shared `phpwarden-performance` skill can perform this flow on an explicit
connect/disconnect request for the selected project and client. It reads its
packaged connection reference, reuses these commands and reports configuration
and actual tool availability separately. Reading captures alone does not request
configuration changes; missing setup never triggers automatic SPX installation.

### Manual configuration

Replace the executable, reports and project paths below with absolute paths on
your computer. MCP argument arrays do not expand `~` like a shell. Use one
connection per checkout and preserve existing client settings. These snippets
are an alternative for manually managed configurations or capture directories
without a PHPWarden runtime receipt.

### Codex

Merge a server block into the trusted project's `.codex/config.toml`. Keep this
machine-specific file locally excluded from Git when it contains local capture
paths, and do not replace an existing shared file:

```toml
[mcp_servers.phpwarden_performance]
command = "/absolute/path/to/phpwarden"
args = ["performance", "mcp", "--reports", "/absolute/path/to/reports", "--producer", "62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07", "--project", "/absolute/path/to/project", "--container-root", "/var/www/html"]
startup_timeout_sec = 10
tool_timeout_sec = 20
```

Codex also supports personal configuration in `~/.codex/config.toml`, but that
scope makes the same fixed capture directory available across projects. Prefer
the project-scoped entry for checkout-specific reports. See [Codex MCP
configuration](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

### Cursor

Merge the server into the project's `.cursor/mcp.json`, with a local Git
exclusion for machine-specific connection settings:

```json
{
  "mcpServers": {
    "phpwarden-performance": {
      "command": "/absolute/path/to/phpwarden",
      "args": ["performance", "mcp", "--reports", "/absolute/path/to/reports", "--producer", "62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07", "--project", "/absolute/path/to/project", "--container-root", "/var/www/html"]
    }
  }
}
```

Keep other `mcpServers` entries intact. A personal `~/.cursor/mcp.json` entry
instead exposes that fixed connection in all projects. See [Cursor MCP
configuration](https://cursor.com/docs/mcp).

### Claude Code

From the project directory, add a private project-local connection:

```sh
claude mcp add --transport stdio --scope local phpwarden-performance -- \
  /absolute/path/to/phpwarden performance mcp \
  --reports /absolute/path/to/reports \
  --producer 62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07 \
  --project /absolute/path/to/project --container-root /var/www/html
```

Claude stores local-scope MCP connections under that project in its personal
configuration. This avoids creating a shared `.mcp.json` with local paths.
See [Claude MCP scopes](https://code.claude.com/docs/en/mcp#mcp-installation-scopes).

## Tools and workflow

| Tool | Inputs | Result |
| --- | --- | --- |
| `find_reports` | Optional `offset`, `limit` | Validated metadata, newest first, plus diagnostics for malformed siblings |
| `get_report_metadata` | `report_key`, optional `snapshot` | Metadata and capture fingerprint after full body validation |
| `get_flat_profile` | `report_key`; optional `snapshot`, `function_id`, `function_contains`, `sort`, `offset`, `limit` | Function IDs, calls, self/inclusive ms and safe definition locations |
| `get_callers` | `report_key`, `function_id`; optional `snapshot`, `offset`, `limit` | Direct callers and aggregate call counts |
| `get_aggregated_call_graph` | `report_key`; optional `snapshot`, `function_id`, `depth`, `offset`, `limit` | Direct-call edges and global per-function node measurements |

All input objects reject unknown fields. Keys are report identifiers rather
than paths, function filters are literal substrings, sort is `self_ms`,
`inclusive_ms` or `calls`, page limits are 1–50, and graph depth is 1–5.
Function IDs are local to one capture. Caller ID -1 means request/command entry.
Omitting the graph focus starts from entry; supplying a function focuses the
bounded reachable graph. Cycles can represent recursion. Aggregate edges do
not establish context-specific call paths or per-edge durations.

Start with `find_reports`, obtain a snapshot from `get_report_metadata`, and
carry it into later calls. Continue with `next_offset` while preserving other
query arguments. `total`, `returned`, `omitted` and `next_offset` describe the
current view's rows, not the full raw event count. A graph page counts edges;
its function array describes the visible endpoints. Output limits can make a
page shorter than requested; the next offset still advances by returned rows.

Every tool returns structured JSON and a JSON text fallback. Incomplete input,
stale snapshot, unavailable data, invalid selection and busy queries set MCP
`isError: true`; inspect `complete`, `exit_code` and diagnostics. A partial
directory listing can retain valid rows with explicit sibling failures. An
explicitly selected capture can succeed independently. Focused queries never
return partial function/graph data when full capture validation fails.

Times are milliseconds. Memory is the whole capture peak, not per-function
allocation. Non-monotonic producer-adjusted CPU data is omitted with
`cpu_omitted_reason`. Definition locations are not call sites, and path mapping
does not authenticate the current source revision. Captured names/descriptions
are untrusted data and can still contain application information despite
redaction. Review before sharing. Measurements of one capture alone do not
prove a regression, optimization, SQL query or N+1 problem.

## Limits and lifecycle

The native reader limits still apply: 25 reports per directory query, bounded
input/decoded bytes, one million events, 8,192 functions, 32,768 direct edges,
512 stack frames and a 15-second query deadline. A selected-report query does
not silently skip broken records within that capture.

MCP adds a 64 KiB inbound-frame cap, an 8 MiB/2,048-frame input budget per stdio
session, a 128 KiB structured query-output cap and a 512 KiB outgoing-write cap.
Function/graph rows share a 64 KiB page budget. Only one expensive query runs at
a time; concurrent callers receive a `busy` tool error and can retry. Restart
the client connection when the session budget is exhausted. EOF or cancellation
closes the session; it does not stop profilers or delete recordings.

## Agent skill

`phpwarden init --agents all -n` installs `phpwarden-performance` alongside the
other shared skills. Invoke `$phpwarden-performance` in Codex or
`/phpwarden-performance` in Cursor/Claude Code after configuring MCP. The skill
analyzes existing captures and relevant source; recording new scenarios and
runtime changes remain separately authorized actions. Default quality checks
and completion hooks do not run performance profiling.

## Validation

Tests use the official Go MCP client for discovery, typed schemas, structured
outputs, all five tools and stdio lifecycle. Hostile inputs cover arbitrary
paths/commands, invalid IDs/ranges, truncated focused captures, stale tokens,
symlinks/directory replacement, malformed sibling metadata and protocol limits.
The CLI test runs with an empty PATH and no project configuration.

An independent JSON-RPC stdio probe negotiated `2025-11-25` and exercised all
five tools against the read-only Drupal scratchpad captures. It found 500
`Drupal\Component\Serialization\Yaml::encode` calls, its controlled direct
caller and the mapped definition at `web/core/lib/Drupal/Component/Serialization/Yaml.php:18`.
The Drush capture contained 141,492 calls; unreliable CPU values stayed omitted.
No application runtime was started for these MCP tests. Live Codex/Cursor/Claude
sessions were not modified or used as the test client.
