# Performance profiles

## Read recorded performance profiles (experimental)

`performance` reads existing SPX captures and prints a summary of slow functions, their callers, execution time and overall peak memory. It supports text and JSON. Reading reports needs no PHP, Docker, running application or project configuration.

```sh
phpwarden performance --reports /path/to/spx-reports \
  --producer 62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07
```

Use `--report KEY` to select one capture, `--limit 5` to shorten the function list, or `--format json` for scripts and agents. To show local source locations, add both `--project /path/to/project` and `--container-root /project`, using the root recorded inside the application container. Locations identify function definitions. Mapping paths does not verify that the current source matches the captured code.

`Self ms` is time spent in the function itself; `Total ms` includes its children. Recursive calls do not count the same inclusive interval twice. Durations are milliseconds; memory is the whole capture's peak, not per-function allocations. If the producer's overhead correction makes CPU counters regress, CPU values are omitted with an explicit `cpu_omitted_reason`; validated wall time and call counts remain available. Results describe the recorded request or command, and do not establish a performance regression by themselves.

This first reader supports complete, unsampled gzip or Zstandard captures from the pinned producer above. `--producer` is an explicit operator assertion, not authenticated provenance; the extension version alone cannot select the format or time units. Malformed metadata, missing bodies, unsupported/sampled/filtered recordings, mismatched counts, unbalanced events and changed inputs produce an incomplete report with exit 3. Directory mode validates all discovered report pairs; `--report` explicitly narrows that scope. Invalid arguments return 2, cancellation 130, and complete reading 0. These are performance results, separate from the static-check report schema.

The reader rejects input symlinks, confines source mapping to the selected workspace, omits host/user/custom metadata and CLI arguments, and strips HTTP query strings, fragments and userinfo. Request paths and function names remain visible; inspect the output before sharing it. Inputs are bounded to 25 reports, 64 KiB metadata, 16 MiB compressed and 64 MiB decoded per report, with aggregate 64 MiB input/verification and decoded budgets. Other limits are one million events, 8,192 functions, 32,768 caller edges, 512 stack frames, 64 KiB lines and 15 seconds per invocation. Summaries show at most 20 functions and three direct callers per function, with omitted-function counts. JSON output stays below 2 MiB.

Reading reports installs nothing and does not record requests. Local runtime setup is described below; agents can use the [read-only MCP server](../tools/spx/MCP.md) and the optional `phpwarden-performance` skill. Profiling remains separate from `check` and agent completion hooks. See [the SPX fixtures and probe](../tools/spx/README.md).

### Agent access through MCP

```sh
phpwarden performance mcp --reports /path/to/spx-reports \
  --producer 62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07 \
  --project /path/to/project --container-root /var/www/html
```

An MCP client starts this command as a stdio subprocess. It exposes five read-only tools: `find_reports`, `get_report_metadata`, `get_flat_profile`, `get_callers` and `get_aggregated_call_graph`. They use the guarded native reader, validate the entire capture before returning a focused view, and carry explicit completeness, diagnostics, pagination and snapshot fingerprints. The graph aggregates direct-call counts; function timings apply across the whole capture. Query arguments cannot select arbitrary files or execute commands.

The server needs no PHP, Docker or network and starts no profiler. After `performance setup`, install a local connection using its saved capture paths:

```sh
phpwarden performance connect --project /path/to/project --client all
phpwarden performance disconnect --project /path/to/project --client all
```

Choose `codex`, `cursor`, `claude` or a comma-separated selection instead of `all`. Repeat `--state-dir` if setup used a custom state base. New Codex/Cursor config files are locally excluded from Git; tracked configs and existing unignored files are refused. Claude uses its private project-local configuration. Existing settings and servers are preserved; disconnect removes only unchanged owned entries and keeps captures. Reload the client after connecting and trust the project in Codex. [MCP setup details](../tools/spx/MCP.md) include manual configuration. `init --agents all -n` installs the shared performance skill and the usual Claude alias separately.

The `phpwarden-performance` skill also handles an explicit request such as
"Connect PHPWarden performance MCP for this project in Codex" or "Disconnect it
for Cursor". It selects the requested client and saved project setup, then uses
the same host CLI commands above. Missing saved setup is reported as a prerequisite;
connecting does not implicitly install SPX, build images or start recording.
Configured connections and tools actually available in the current client are
reported separately. A performance review alone does not change client settings.

### Local profiler setup (experimental)

Prepare a local SPX integration, inspect it, then explicitly build and activate it:

```sh
phpwarden performance setup --project /path/to/project
phpwarden performance setup --project /path/to/project --apply
phpwarden performance status --project /path/to/project
```

Runtime management uses the host CLI and a local Docker Unix socket. DDEV is detected from `.ddev/config.yaml` and requires a Git checkout. Setup creates four new, locally excluded adapter files under `.ddev`; existing files, tracked runtime configuration, `.gitignore` and `.phpwarden.yaml` are preserved. Private settings and reports live outside the checkout, under the user's cache directory. Linked worktrees have separate runtime state. Source is downloaded from the pinned SPX commit and verified by SHA256; use `--spx-archive FILE` for offline preparation. Activation may still need package repositories for build dependencies.

For Compose, explicitly select the PHP service and pass the original files, env files and profiles in their original order. The existing local service image is resolved to its immutable ID; an explicit `--image REF` can select another existing local base. No original application image is built automatically:

```sh
phpwarden performance setup --project /path/to/project \
  --environment compose --php 8.3 --service php \
  --compose-file compose.yaml --compose-file compose.dev.yaml \
  --env-file .env.dev --profile dev --container-root /app
phpwarden performance setup --project /path/to/project --apply
```

The overlay and build context stay outside the repository. Compose project name, project directory, file order, env files, profiles and original service settings are preserved. Only the selected service receives the profiler image, private INI and reports mount. Resolved Compose settings are fingerprinted without persisting raw environment values; changed inputs require removing the old integration and preparing it again. Existing compatible development images with official-PHP `phpize`/`php-config` and Debian or Alpine build prerequisites are supported. The current runtime target is Linux amd64 NTS PHP 8.1–8.4; DDEV uses its versioned PHP development packages.

`--apply` invokes `ddev --skip-hooks restart` for DDEV, which restarts that project's services. For Compose it builds the local derivative and runs `up --no-deps --no-build --pull never` for the selected PHP service. Setup without `--apply` prepares files without building or restarting containers. `status` probes the selected PHP CLI process and verifies its producer marker, configuration fingerprint and mounts; it does not start containers. `runtime_checked` distinguishes a probe from a preparation-only result. It does not claim that an HTTP/FPM request has been exercised.

Bare Docker uses `--environment docker --container NAME --php 8.3 --container-root /app`. `setup --apply` builds the derivative image and returns `launch_arguments` but never clones or recreates the selected container. Use those arguments and image with the original mounts, networks, ports, user, entrypoint and command. Restore the original launch manually before teardown. This preserves settings that cannot be reconstructed safely from a generic container inspection.

After activation, explicitly profile a CLI command, for example:

```sh
ddev exec env SPX_ENABLED=1 SPX_REPORT=full SPX_METRICS=wt,ct,zm \
  SPX_SAMPLING_PERIOD=0 vendor/bin/drush status
```

Normal requests and commands remain unprofiled. HTTP support requires `setup --http`; it generates a private random key and defaults to loopback clients. Use `--http-allow IP_OR_CIDR_LIST` for a specific local proxy/client range. Send `SPX_ENABLED=1` and `SPX_KEY` as cookies, keeping the key out of URLs and reports. The key's local file path and a ready-to-copy report-reading command appear in setup/status output. Raw captures can contain application data and remain local.

```sh
phpwarden performance teardown --project /path/to/project --apply
```

Teardown validates ownership hashes and removes only generated adapters and its own Git exclusion block. Edited files are preserved and reported as conflicts. Without `--apply`, teardown removes adapters but does not unload SPX from an existing container; restore/restart the original runtime separately. DDEV/Compose `--apply` also restores the original runtime. Bare Docker restoration is manual. Recordings and local images are retained. All lifecycle commands support `--format json`; `--state-dir DIR` selects a private state base outside the project. See [runtime validation](../tools/spx/RUNTIME.md).
