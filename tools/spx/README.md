# SPX/MCP compatibility spike

This directory records the compatibility experiment and fixtures for the experimental native report reader.
It supplies pinned source identities, two sanitized real SPX v0.5-format profiles,
their controlled PHP scenario and a standalone reader probe.

The fixture runs the same expensive computation 24 times or caches its result
and runs it once. No application data is included. Function locations are
definition locations; they are not the individual call sites.

## Run the reader probe

Read the sanitized captures through PHPWarden without PHP, Docker or MCP:

```sh
go run ./cmd/phpwarden performance --reports tools/spx/fixtures \
  --producer 62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07 \
  --project tools/spx/fixtures --container-root /fixture
```

Add `--format json` for the bounded, redacted report. The native reader validates
the complete stream before exporting function summaries. The producer flag is
an operator assertion; it does not authenticate a capture. It accepts only the
verified complete unsampled format, including explicit recorded/sampling counts.
Older upstream fixtures lacking those fields are rejected rather than inferred.
This command does not install SPX, record requests or expose an MCP server.

The following probe still tests the raw upstream MCP implementation and its
known limitations, independently of the native reader:

Use the exact MCP archive in `sources.json`, verify its SHA256, extract it outside
the repository and install its locked dependencies with plugins/scripts disabled
using PHP >=8.3. The probe checks the lock identity and expects an already present
local Docker image containing PHP >=8.3 and zlib. It downloads no image or package.

```sh
rtk proxy python3 tools/spx/probe.py \
  --reader-image 'ghcr.io/rchehd/phpwarden@sha256:9f41438baea5090c7d8d1b25a89fd41c82aef71422734071293a9e5972f4b573' \
  --mcp-dir /absolute/path/to/pinned/php-spx-mcp \
  --output /tmp/phpwarden-spx-probe.json
```

The probe uses a private temporary report directory and a disposable container
with read-only mounts, no network and no Docker socket inside it. It does not
mount the user's application. Its only synthetic symlink targets the reader's
own public Composer manifest. The daemon must use a local Unix socket.

Exit 0 means the experiment completed and reproduced the expected function
counts, source locations and all five MCP tools. Read `ready_for_integration`:
it remains false when upstream behavior needs an adapter. Protocol failures,
wrong counts/locations and missing prerequisites fail the command. This script
is a test instrument; it does not provide redacted access to production reports.

## Results, 2026-10-27

Actual SPX writers passed on Linux amd64 NTS PHP 8.1.34, 8.2.35, 8.3.36 and
8.4.26. PHP 8.2 used glibc; the other three used musl. Each generated gzip
profiles for three repeated and three cached runs. A PHP 8.3.36 reader found the
hotspot, its caller and `/fixture/scenario.php:2` in all 24 profiles.

Separate HTTP and Drush fixtures used a sanitized Drupal 11.4.1 copy. Both
recorded and exposed 500 calls to `Drupal\Component\Serialization\Yaml::encode`
and the controlled caller. The Drush command used bootstrap NONE, and the HTTP
request loaded core components through Composer. No database-backed Drupal
installation or live DDEV service was tested.

All 58 upstream tests passed (2692 assertions) when the isolated reader loaded
the pinned zstd extension. All five MCP tools also read the upstream `.zst`
fixture. Native writers in this matrix generated gzip; native SPX zstd output
was not separately verified. ARM64, ZTS and other PHP/base-image combinations
remain unverified. No Docker image was built or published; SPX compilation ran
inside disposable containers and left its artifacts outside this repository.

## Required before product integration

- For the pinned producer, raw `wt`/`ct` graph values are nanoseconds, while the
  metadata field named `wall_time_ms` contains microseconds. Normalize by a
  verified producer identity; do not label or filter raw metadata as milliseconds.
- Constrain metadata and bodies to the selected report directory. Upstream
  rejects `../` keys, but follows a metadata symlink outside that directory.
- Validate complete, balanced event streams before exposing any view. A full
  graph rejects the tested truncated stream, while a focused subtree succeeds
  and does not carry a machine-readable incomplete flag.
- Surface malformed metadata explicitly. Upstream listing omits it with a
  stderr warning, so an empty/smaller list is not proof of complete coverage.
- Bound compressed/decompressed input, memory, runtime and output; redact URI
  arguments and map container paths to the chosen workspace before agent access.

The native `performance` reader implements these guards and uses its own Go
gzip/Zstandard decoder instead of exporting raw MCP results. Its tests include
malformed streams, recursion, resource limits, redaction and path confinement.
Application setup follows separately and must remain local for each checkout:
DDEV, Compose and bare Docker adapters, with no tracked project-file edits.
No setup command or default checker is introduced by the reader.

Native-reader validation replayed all 24 PHP 8.1–8.4 captures, the HTTP capture
and three Drush captures from the spike without PHP or Docker. Source mapping
was also checked against the sanitized Drupal copy. Go tests cover both codecs,
recursion, invalid metadata/events, missing/ambiguous pairs, symlinks, input
changes, bounded summaries, cancellation and hostile terminal characters.
