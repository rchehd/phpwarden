# Performance dependency inventory

The shipped native MCP server uses the official Go MCP SDK. Its dependency
versions, module checksums, license-file hashes and complete upstream notices
are generated from the actual Linux amd64, CGO-disabled CLI dependency graph:

```sh
python3 tools/licenses.py check
python3 tools/licenses.py generate
phpwarden --licenses
phpwarden --licenses --format=json
```

`check` verifies downloaded Go module contents and compares the generated
inventory with the committed copies. Adding an unversioned/replaced module,
missing license material, or changes to versions/notices fails this gate.
`generate` is the deliberate update step after reviewing dependency changes.
The Go toolchain license is included as well as all discovered module license
material, including notices for bundled subcomponents.

The CLI embeds these notices so they remain available when only the executable
is installed or updated. Standalone, Composer and npm archives also include
`THIRD_PARTY_NOTICES.txt` and `dependencies.json`. The packager reads both from
the supplied executable rather than substituting its own dependency versions.
The installer/updater allow these bounded regular archive entries; legacy
archives without them remain readable.

SPX is a separate optional application-runtime extension. Its immutable commit,
source URL, SHA256 and recorded upstream license identifier are in
[`sources.json`](sources.json). The generated runtime image retains:

- `/opt/phpwarden-spx/LICENSE`: the original upstream license text.
- `/opt/phpwarden-spx/source.tar.gz`: the exact verified source archive.
- `/opt/phpwarden-spx/source.sha256`: its SHA256.
- `/opt/phpwarden-spx/build.sh`: the build procedure used by PHPWarden.
- `/opt/phpwarden-spx/producer`: the exact producer commit.

Source and build material are readable by the application user. The generated
Dockerfile pins the original local base image, and the host's private runtime
receipt retains that binding. Building the extension does not change the
normal `check`/completion-hook pipeline or make profiling automatic.

The raw upstream PHP MCP implementation, its Composer packages and PECL zstd
extension are compatibility-spike inputs only. They are neither embedded nor
launched by the native CLI. Their historical pins remain explicitly labelled
in `sources.json`; current Go dependencies are in
[`../../internal/legal/dependencies.json`](../../internal/legal/dependencies.json).

This inventory covers the CLI and SPX integration. Existing PHP/Node analyzers
and base-image software retain their own license material and are not claimed
to be covered by this CLI inventory. The project has not assigned a new
PHPWarden license through this change. Upstream materials are available from
[SPX](https://github.com/NoiseByNorthwest/php-spx/tree/62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07)
and the [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk/blob/v1.8.0/LICENSE).
