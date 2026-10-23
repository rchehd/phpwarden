# PHPWarden

PHP quality checks for local development, CI, and coding agents.

PHPWarden is being designed as a standalone CLI with Docker-based tools and adapters for Claude Code and Codex. Drupal will be the first framework profile; additional PHP frameworks can follow through separate profiles.

## Planned capabilities

- Run quality tools from versioned Docker images without requiring host PHP or Composer.
- Check changed code by default, with full-project and explicit-path modes.
- Share project-specific settings through `.phpwarden.yaml` across CLI, CI, and agent integrations.
- Extend the tool through modular checkers, framework profiles, and runners.
- Distinguish code findings from unavailable tools and execution failures.

## Development status

The project is in early development. The Go CLI validates configuration, discovers Drupal layout and locked versions, inspects Git scope, selects a development toolset, and runs PHPStan/phpstan-drupal, PHPCS/Drupal Coder, PHP debug, PHP syntax, YAML syntax, Drupal module config, whitespace, and conflict marker checks through Docker. A local development image contains PHP 8.4, Git, and isolated Symfony YAML dependencies. Published images, automatic pulls, and agent integrations are not available yet.

## Build and try the CLI

Requires Go 1.26.5. From the repository root:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
bin/phpwarden config validate --config examples/drupal.phpwarden.yaml
bin/phpwarden config show --config examples/drupal.phpwarden.yaml --format json
```

For a project, save its configuration as `.phpwarden.yaml` in the project root. The CLI finds the nearest configuration from the current directory up to the Git worktree root, or accepts an explicit `--project` or `--config` path. Explicit paths also allow configuration validation outside Git. Relative command-line paths are resolved from the caller's directory.

Validation checks YAML structure, the [v1 schema](schemas/config-v1.schema.json), PHP target ranges, and path boundaries. It rejects unknown fields and checker IDs, duplicate keys, aliases, and numeric PHP versions. The implemented checkers are `debug`, `drupal-config`, `phpcs`, `phpstan`, `syntax`, `whitespace`, and `yaml`, all enabled by default for the Drupal profile. PHPCompatibility is an opt-in PHPCS setting.

`config show` includes discovered project facts, core and Drupal defaults, explicit project overrides, and their provenance. Its `pending` list identifies unresolved values. `config validate` checks configuration syntax/schema and path boundaries without inspecting Composer metadata. Discovery validates Composer PHP requirements and compares installed Drupal version metadata when available. Full dependency integrity and release image compatibility remain future work. The development launcher currently requires PHP runtime and target `"8.4"`.

Configuration commands return exit `0` for valid input, `2` for usage/configuration errors, and `3` for output failures. With `--format json`, stdout contains a single JSON document. These configuration responses are separate from the worker's code-check report.

## Inspect analysis scope

In a Git project with `.phpwarden.yaml` and discovered or explicit source roots:

```sh
phpwarden scope --base origin/main --format json
phpwarden scope --mode full
phpwarden scope --base origin/main --whole-files
phpwarden scope web/modules/custom/demo
```

The diff scope compares the final working tree with the merge base and includes committed, staged, unstaged, and nonignored untracked changes. Without an explicit base, it uses `scope.base`, then the locally available `origin/HEAD`; it never guesses a branch or fetches history. Full scope selects tracked and nonignored untracked files within the configured source roots. Explicit paths are relative to the caller's directory and still respect roots and exclusions.

JSON `scope.files` contains project-relative paths. `scope.line_map` maps each path to inclusive `{start, end}` ranges; `null` means all lines, while `[]` means a selected file has no added lines. New files and rename destinations select all lines. Deleted files are omitted. `--whole-files` expands reporting to all lines of changed files and does not select unchanged files.

Scope inspection uses discovered Drupal custom directories when `scope.include` is omitted. Explicit includes replace those defaults; core/contrib/vendor exclusions remain additive even for `include: [.]`. If no custom roots can be discovered, set explicit existing paths. Symlinks and nested submodules are not traversed. Non-UTF-8 filenames and unavailable Git history produce errors. This command inspects scope and does not run code checkers or declare the code clean.

## Drupal discovery

Discovery reads files and directory metadata without loading Drupal, running Composer scripts, installing dependencies, or accessing a database. It runs for `config show`, `scope`, `resolve`, and both check commands. Use `config show --format json` to inspect `effective.project`, resolved configuration, provenance, and pending fields.

The framework version comes from exact `drupal/core` or `drupal/core-recommended` entries in `composer.lock`, with installed core metadata as a fallback when the lock does not supply a version. A leading `v` is normalized. Conflicting metadata or a conflict with explicit `framework.version` are errors. Constraints, development branches, and prerelease versions are not guessed as stable versions. Without a usable discovered version, an explicit exact version is accepted with `project_config` provenance; otherwise the version stays pending. The current framework-independent static toolset can run with a pending version; future framework-dependent adapters will require verified compatibility.

The web root comes from Composer's `extra.drupal-scaffold.locations.web-root`, then `extra.installer-paths` for `type:drupal-core`. Otherwise a unique root, `web/`, or `docroot/` layout is recognized through `core/lib/Drupal.php` or custom directory conventions. Conflicting or ambiguous layouts return an error. These metadata fields follow [Drupal's Composer configuration](https://www.drupal.org/docs/develop/using-composer/manage-dependencies).

Default source roots are existing `modules/custom`, `themes/custom`, and `profiles/custom` directories, including `sites/*` variants. Literal custom installer directories ending in `/{$name}` are supported. Core, conventional contrib directories, and Composer's custom `vendor-dir` are excluded. Contrib installer paths outside a `contrib` directory exclude individual typed locked packages, preserving adjacent custom code. Installer templates containing other variables require a supported literal mapping; arbitrary Composer plugin layouts are not resolved yet.

Project paths remain relative to the configuration root. Discovery rejects path traversal, symlink traversal, duplicate JSON keys, and malformed relevant metadata. Composer file hashes and layout facts are compared before/after analysis, including missing metadata files and newly appearing custom/multisite directories; changes make the result stale.

## Composer compatibility

Discovery reads root `require.php`, `require-dev.php`, `conflict.php`, lock platform requirements, and locked package PHP requirements/conflicts. Production requirements constrain both tool runtime and the entire configured target minor range. Development requirements constrain runtime, allowing production code to target an older PHP than its development dependencies need. This check does not inspect extensions or solve the dependency graph.

The Go parser supports stable numeric constraints with comparisons, AND/OR, hyphen ranges, wildcards, caret, and tilde syntax. Unsupported branch/stability/alias expressions fail explicitly. Its stable-version behavior is checked against 660 precomputed results from [composer/semver 3.4.4](https://github.com/composer/semver/tree/3.4.4); running tests needs no Composer installation. See [Composer's constraint syntax](https://getcomposer.org/doc/articles/versions.md).

Each configured target minor must have at least one patch satisfying all production constraints. If only some patches qualify, `php.target.patch` stays pending. A runtime minor with patch restrictions similarly adds `php.runtime.patch` during discovery. `resolve` checks the selected image's exact PHP version; the worker reports `environment.php_version` and checks the observed version again. Runtime patch uncertainty is removed after that check; remaining compatibility pending fields appear in JSON and text reports. These fields do not mean all target patches have been tested.

`config.platform.php` is recorded as Composer's simulated dependency-resolution version, checked against declared requirements, and kept separate from PHPWarden's runtime. It never replaces `php.runtime`. Lock `platform-overrides`, extension/platform packages, and lock content-hash validation are not evaluated yet. [Composer documents the platform setting here](https://getcomposer.org/doc/06-config.md#platform).

When available, `<vendor-dir>/composer/installed.json` is read as Composer 1's package array or Composer 2's package object. `effective.project.installed` reports `absent`, `not_installed`, `unverified`, `observed`, or `verified`. Verification compares Drupal version metadata with the lock; it does not verify installed source files or all package versions. A no-dev installation may omit core when core appears only in the lock's dev packages. Missing vendor remains valid for the debug checker. Existing conflicting, incomplete required-core, malformed, or unsafe installed metadata produces an input error.

Project autoload and executable `installed.php` are never loaded. If only PHP metadata exists, its presence is reported as unverified and `framework.installed_version` stays pending. Installed JSON content/presence joins freshness checks, so changes during analysis invalidate the result even though vendor is excluded from code scope.

## Debug worker

The worker command runs directly in its execution environment. The debug checker requires PHP with the tokenizer extension; its PHP minor version must match `php.runtime`. It can run inside a preselected image or directly during development. Public `phpwarden check` uses Docker and has no host PHP fallback.

In a configured project with discovered or explicit source roots:

```sh
phpwarden worker check --only debug --base origin/main --format json
phpwarden worker check --mode full
phpwarden worker check --whole-files
phpwarden worker check web/modules/custom/demo
```

The checker tokenizes PHP source without executing it or loading project dependencies. Defaults prohibit `var_dump`, `print_r`, `dpm`, `dsm`, and `kint`; `tools.debug.functions` replaces that list. It recognizes direct calls, including multiline calls and leading backslashes, and ignores comments, string literals, declarations, object/static methods, attributes, and first-class callable acquisition. Variable calls, imported function aliases, and full namespace/shadowing resolution are not covered. It does not check PHP syntax or JavaScript debug statements.

`tools.debug.enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude` control execution. `--only debug` explicitly enables the checker even when disabled in configuration. Reporting uses the shared Git line map. No project `vendor` directory is required.

## Check PHP syntax

`syntax` runs `php -l` separately for each selected PHP/Drupal file, without executing the source or loading project dependencies. It reports parser and compile errors across the entire selected file, including unchanged lines and files changed only by deletions. Error positions can point to the end of a file. Missing executables, timeouts, resource failures, and unrecognized tool output produce execution errors rather than a clean result.

```sh
phpwarden check --only syntax
phpwarden check --only debug,syntax --mode full
```

Settings live under `tools.syntax`: `enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, `exclude`, and `extra_extensions`. The timeout covers the complete syntax run. Default extensions are `.php`, `.module`, `.inc`, `.install`, `.theme`, `.profile`, `.engine`, and `.test`; `extra_extensions: [".phtml"]` adds to that list. `fail_on: never` keeps findings visible without blocking. Explicit `--only` enables the selected checkers and excludes all others; unknown or duplicate IDs are input errors.

Lint uses the image's PHP runtime, currently 8.4.26. It does not prove compatibility with every configured PHP target, resolve missing functions/classes, or check interactions between separately compiled files. Deprecation and warning checks belong to other adapters. No site, database, Composer installation, or project `vendor` directory is needed.

## Check YAML syntax

`yaml` parses every selected `.yml` or `.yaml` file with Symfony YAML 7.4.21, including uppercase extensions. Errors anywhere in the selected file are reported, including unchanged lines and deletion-only changes. An error without a parser position, such as invalid UTF-8, becomes a file-context global finding instead of an invented line number.

```sh
phpwarden check --only yaml
phpwarden check --only debug,syntax,yaml --mode full
```

Use `tools.yaml.enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude`. The checker timeout covers the entire helper run. `--only yaml` enables it explicitly; `fail_on: never` retains findings without blocking. Missing bundled dependencies, missing PHP, crashes, malformed reports, and timeouts are execution failures. Optional failures remain incomplete.

The parser uses a fixed preset: custom tags are accepted as data, collection alias resolutions and nesting depth are each limited to 128, and PHP object deserialization and constant/enum resolution are disabled. Project autoload, scripts, application code, and databases are never loaded. PHP-specific tags are not evaluated or validated against project symbols. Symfony's supported YAML subset excludes multi-document files; a syntax pass does not validate Drupal services, routes, or configuration schemas.

Dependencies are isolated in [tools/yaml/composer.json](tools/yaml/composer.json) and its lock file. The image installs them during a separate build stage with scripts/plugins disabled; Composer itself is not shipped in the runtime image. Native `worker check` expects its tool bundle at `../lib/phpwarden/yaml/vendor/autoload.php` relative to the worker binary. The Docker image provides that layout; a standalone host launcher needs no local PHP, Composer, Symfony, or project `vendor`.

Reports follow the [v1 report schema](schemas/report-v1.schema.json), which is embedded and validated before output. Exit codes are `0` for no blocking findings, `1` for blocking findings, `2` for input errors, and `3` for required checker/process failures or changed inputs. Optional failures return `complete: false` and a visible `PARTIAL` text result. The worker checks file/configuration checksums and recomputes scope after analysis. Each helper run owns and removes its temporary directory.

Reports include `environment.framework_version` when an exact version is available. Direct `worker check` reports have no resolved image/toolset metadata. Public `check` reports identify the inspected immutable local image ID and development toolset. Release compatibility validation remains pending.

## Check whitespace and conflict markers

`whitespace` checks all selected file types for trailing ASCII spaces/tabs and unresolved merge conflict markers. Diff mode reports added lines only. Full mode and explicit paths check all lines; `--whole-files` checks all lines of changed files, including deletion-only changes.

```sh
phpwarden check --only whitespace
phpwarden check --only whitespace --mode full
```

Settings live under `tools.whitespace`: the common `enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude` fields, plus independent `trailing_whitespace` and `conflict_markers` toggles. `trailing_whitespace_exclude` defaults to `["**/*.md", "**/*.patch", "**/*.diff"]`; `conflict_markers_exclude` defaults to `[]`. Rule-specific lists replace defaults, so set `trailing_whitespace_exclude: []` to check Markdown trailing spaces. Common exclusions apply to both rules. Patterns are case-sensitive, as with other checker exclusions.

CRLF line endings are accepted; trailing spaces/tabs before CRLF or at EOF are findings. Files containing a NUL byte anywhere are skipped as binary; other content is checked as bytes, without requiring UTF-8. Columns are one-based byte positions. Marker detection recognizes runs of at least seven identical `<`, `|`, `=`, or `>` characters at column one, followed by a space, tab, or end of line. It flags each marker independently, including diff3 base markers and longer marker widths. Indented markers, quoted patch lines, and shorter runs are not recognized. A literal separator such as `=======` also matches; use marker-specific exclusions for intentional examples.

Findings use rules `whitespace.trailing` and `conflict.marker`, with `kind: code`. The implementation runs inside the Go worker without launching Git, PHP, or project code. Public `check` continues to use the selected Docker image. No fixes are applied.

## Check Drupal module configuration

`drupal-config` rejects top-level `uuid` and `_core` keys in selected module configuration files. It checks the entire selected file, including unchanged lines and deletion-only changes. Nested keys, comments, and text values are allowed. Quoted, flow-style, and escaped keys are recognized; keys inherited through a top-level YAML merge are reported at the merge key. Explicit keys take their own position.

```sh
phpwarden check --only drupal-config
phpwarden check --only drupal-config --mode full
```

The default `directories` are `config/install`, `config/optional`, and `config/schema`, relative to a module root. Matching includes files in their subdirectories and accepts `.yml`/`.yaml` case-insensitively. Module roots are inferred from conventional `modules/<name>` and `modules/custom/<name>` or `modules/contrib/<name>` paths, including multisite and nested modules. Scope and core/contrib exclusions still apply. This is path-based classification; module metadata is not loaded. Site config exports, unrelated YAML, themes, and profiles are outside this default classification.

Settings live under `tools.drupal-config`: `enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, `exclude`, `directories`, `forbidden_keys`, and `module_roots`. `directories` and `forbidden_keys` replace their defaults; `forbidden_keys: []` disables the rules. `module_roots` adds exact project-relative module directories for custom layouts. For a standalone module repository, set `module_roots: ["."]` and an appropriate `scope.include`; a Composer custom installer directory likewise needs explicit module roots. Roots are literal paths, not glob patterns. They never expand the shared file scope. Use include/exclude to narrow conventional modules.

The checker uses the Go worker's existing YAML parser (`go.yaml.in/yaml/v3`) to inspect mappings, without PHP, Drupal bootstrap, project autoload, or a database. It accepts one document, limits syntax-tree depth to 128, and uses the parser's alias validation/expansion safeguards. Malformed YAML, duplicate keys, invalid encoding/aliases/merges, excessive nesting, and read errors make the semantic check incomplete; required errors return exit 3 even with `fail_on: never`. The independent Symfony `yaml` checker supplies syntax findings. This checker does not validate Drupal configuration schemas or require a mapping for otherwise valid empty/scalar/sequence documents.

Findings use `drupal.config.forbidden_key` with `kind: code` and parser line/column positions. All top-level key checks apply to the complete selected file. Public `check` runs this native checker inside the selected image; it applies no fixes.

## Check PHPCS, Drupal Coder, and PHP compatibility

`phpcs` uses PHP_CodeSniffer 4.0.4 and Drupal Coder 9.0.1 from an isolated tool bundle. The Drupal fallback selects `Drupal` and `DrupalPractice`. Files are analyzed in full, then code findings are filtered through the shared added-line map; full/paths/whole-files expose existing style issues too. Sniff exceptions, tokenizer failures, missing packages, invalid rulesets, and malformed reports are execution errors, including when the failure occurs outside a diff hunk. Native warning/error severity and sniff source IDs are preserved.

```sh
phpwarden check --only phpcs
phpwarden check --only phpcs --mode full
```

Use `tools.phpcs.config` for an explicit project-relative ruleset. Otherwise `discover_config: true` chooses the nearest `.phpcs.xml`, `phpcs.xml`, `.phpcs.xml.dist`, or `phpcs.xml.dist`, in that order, from each source directory up to the project root. Files with different rulesets run in separate processes. Project rulesets replace the fallback standards; PHPWarden does not append Drupal rules to them. Set `discover_config: false` to use fallback standards regardless of nearby configuration. No native config file is generated in the project.

Settings include the shared `enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude`, plus `standards`, `sniffs`, `exclude_sniffs`, `extra_extensions`, `error_severity`, and `warning_severity`. Standard names are identifiers such as `Drupal` or `PSR12`. Sniff lists select/exclude installed rules; they apply to project rulesets as well as fallback. Severity thresholds range from 0 to 10; defaults are 5, and 0 disables that message type. PHP/Drupal extensions are selected by default; extra extensions add to this set. An unknown standard or an empty effective sniff selection is an execution error, rather than a clean scan.

`standard_paths` adds project-relative directories containing custom standards; referenced PHP sniffs must be installed there or accessible through the ruleset. Paths containing commas are unsupported for this native option. XML ruleset file/standard references and `<autoload>` paths must resolve within the project, without symlinks; XML directives such as DTDs and dynamic/variable-expanded XML file references are unsupported. Relative XML references may traverse to sibling directories inside the project. Custom sniffs and explicitly declared ruleset autoloaders are executable lint code. The project Composer autoloader and application sources are not loaded automatically; PHPCS source tokenization does not execute the files being checked. Public check uses the same read-only, offline container protections as the other checkers.

PHPCS context fingerprints cover discovered ruleset candidates, including absent candidates, transitive XML/file references, declared autoload files, and all files/directory listings in `standard_paths` and `extra_inputs`. Changes invalidate the result before or after execution. Use `extra_inputs` for additional project-relative files/directories that custom code or native options read indirectly. Arbitrary dynamic PHP dependencies are not inferred, and this does not verify all project dependency integrity. Context preparation errors remain checker errors and preserve required/optional policy.

`tools.phpcs.compatibility.enabled: true` appends PHPCompatibility to the selected ruleset in the same PHPCS invocation. The bundled version is [10.0.0-alpha2](https://github.com/PHPCompatibility/PHPCompatibility/tree/10.0.0-alpha2), so it remains opt-in. `testVersion` is always set from the shared PHP target range, including when a project ruleset selects PHPCompatibility itself. `compatibility.test_version`, such as `"8.3"` or `"8.3-8.4"`, is an explicit override. The PHP container runtime does not replace the target. These sniffs cover their implemented rules, not every possible runtime incompatibility. A project ruleset can select PHPCompatibility independently of the append switch.

Dependencies and all transitive versions are locked in [tools/phpcs/composer.lock](tools/phpcs/composer.lock), installed with Composer scripts/plugins disabled, and registered at runtime without modifying PHPCS configuration. The helper checks pinned PHPCS/Coder/PHPCompatibility versions. Native worker development needs its bundle at `../lib/phpwarden/phpcs` beside the binary plus PHP tokenizer, mbstring, SimpleXML, and XMLWriter extensions. Public `check` requires none of these on the host. Fixing through PHPCBF is not implemented in this step.

## Check PHPStan and Drupal deprecations

`phpstan` uses PHPStan 2.3.1, phpstan-drupal 2.2.2, and phpstan-deprecation-rules 2.0.5 from an isolated tool bundle. Files are analyzed in full, then findings are filtered through the shared added-line map; full/paths/whole-files expose existing issues too. Each finding keeps the PHPStan error identifier, such as `return.type`, as its rule. Non-ignorable errors, global errors, stderr output, malformed reports, and missing identifiers are execution errors, including outside a diff hunk.

```sh
phpwarden check --only phpstan
phpwarden check --only phpstan --mode full
```

Use `tools.phpstan.config` for an explicit project-relative NEON file. Otherwise `discover_config: true` chooses the first of `phpstan.neon`, `phpstan.neon.dist`, or `phpstan.dist.neon` in the project root. PHPWarden writes a temporary configuration that includes the project file, `includes`, and `baseline`; analysed paths always come from the PHPWarden scope. Without a project config, includes, or an explicit `level`, level 3 is used. `level` (0-10) overrides the project level.

Settings include the shared `enabled`, `required`, `fail_on`, `timeout_seconds`, `include`, and `exclude`, plus `level`, `includes`, `baseline`, `bootstrap_files`, `scan_directories`, `stub_files`, `autoload`, `drupal`, `deprecations`, `memory_limit_mb` (default 512), and `extra_extensions`. All paths are project-relative and must be regular files or directories inside the project. The shared PHP target range sets PHPStan `phpVersion`. Diff runs disable `reportUnmatchedIgnoredErrors`, so baselines for unchanged code do not fail.

`drupal: true`, the Drupal profile default, loads phpstan-drupal and requires the project's installed Composer dependencies; without them the check is `unavailable` with `project_dependencies_missing`. `autoload: auto` uses `vendor/autoload.php`, honouring `config.vendor-dir` in `composer.json`; an explicit path replaces it. Unlike PHPCS, PHPStan loads the project autoloader and `bootstrap_files`, so they execute; analysed sources are not executed. `deprecations: false` drops the deprecation rules.

The PHPStan context fingerprint covers every entry under the project root except `.git`: path, type, size, nanosecond modification time, and symlink target, without reading file contents or following symlinks. Any added, removed, resized, retouched, or retargeted entry during the run invalidates the result. An edit that preserves both size and modification time, or a change outside the project behind a symlink, is not detected. PHPStan runs without agent-detection variables such as `CLAUDECODE`, which would otherwise add agent instructions to stderr. The result cache lives in the private temporary directory and is discarded after each run.

Dependencies and all transitive versions are locked in [tools/phpstan/composer.lock](tools/phpstan/composer.lock) and installed with Composer scripts/plugins disabled. The helper checks pinned PHPStan/phpstan-drupal/deprecation-rules versions. Native worker development needs its bundle at `../lib/phpwarden/phpstan` beside the binary. Public `check` requires no host PHP or Composer.

## Check through Docker

After building the Go CLI and development image, run these commands in your configured Git project:

```sh
phpwarden resolve --format json
phpwarden check --base origin/main --only debug --format json
phpwarden check --mode full
phpwarden check --whole-files
phpwarden check web/modules/custom/demo
```

`resolve` reads the compatibility manifest embedded in the CLI, without Docker or network access. Its JSON selection includes the manifest release, entry, preset, tool versions, image, and platform. The bundled manifest currently contains only the development static entry: Linux amd64, profile `drupal`, and PHP runtime/target `"8.4"`. This entry is explicitly independent of Drupal APIs and does not establish a Drupal version support matrix.

The manifest schema is [manifest-v1.schema.json](schemas/manifest-v1.schema.json); the bundled data lives in [internal/toolset/manifest-v1.json](internal/toolset/manifest-v1.json). Release entries specify a framework version constraint, PHP runtime and exact patch, supported target range, versioned preset, toolset and exact tool versions, capabilities, and platform-specific image references pinned as `registry/repository@sha256:…`. Resolution requires one matching entry, including the project's Composer PHP requirements. Missing or ambiguous matches fail; entry order does not determine priority. A toolset/image may serve multiple non-overlapping framework ranges.

Only the tested development image is bundled today. Release-selection tests use synthetic fixtures, which do not advertise actual Drupal/tool compatibility or published images. Registry pulls, real release entries, package locks, and release matrix tests remain pending. Project configuration cannot override the bundled manifest or select an arbitrary image.

`check` computes Git scope on the host, verifies the local image's platform/protocol/toolset/PHP/Symfony YAML labels, and starts it by its inspected image ID. It sends a versioned request with effective settings, selected files/lines, input checksums, and the expected PHP version over stdin. The worker verifies inputs and its actual PHP version; the YAML helper also verifies its Symfony version. The launcher rechecks host checksums and Git scope. Linked worktrees and monorepos work without mounting external Git metadata into the worker.

The launcher requires host Git and a local Docker Unix socket, with no host PHP, Composer, or Go requirement for the built binary. It mounts the project read-only, uses your UID/GID, disables container networking, and gives each run its own temporary filesystem and container name. Interruptions return `130`; global timeouts and runner failures return `3`, with best-effort removal of the run's container. An empty scope or disabling all requested checkers needs no Docker process after configuration/toolset validation.

The development image must already exist under `phpwarden:static-php8.4-dev`; a missing image returns an error with build instructions. There is no registry fallback or pull for this unpublished tag. Remote Docker contexts and other platforms are currently rejected. The worker protocol is now v2, carrying a checker selection list instead of the previous debug-only flag. Rebuild the CLI and image together; old debug images cannot satisfy the new toolset/protocol/capability checks. The report format remains v1, with `environment.protocol: 2`.

The current toolset is `static-dev-v6-php8.4`, with `debug,drupal-config,phpcs,phpstan,syntax,whitespace,yaml` capabilities and preset `drupal-static` 1.5.0. Rebuild the existing development tag after adding PHPStan; earlier images are rejected. The static development entry does not bootstrap Drupal or advertise a tested framework version matrix.

## Run the development Docker image

Build from this repository with Docker; Go and PHP are installed inside the build/runtime images:

```sh
docker build -f docker/Dockerfile -t phpwarden:static-php8.4-dev .
```

From the Git project you want to check, with an existing `.phpwarden.yaml`, run:

```sh
docker run --rm --pull=never --network=none --read-only \
  --cap-drop=ALL --security-opt=no-new-privileges:true \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=256m,mode=1777 \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$PWD,target=/workspace,readonly" \
  phpwarden:static-php8.4-dev worker check --only debug --format json
```

Set PHP runtime/target to `"8.4"` and configure an available diff base. Source roots can be discovered or supplied through `scope.include`. Append `--mode full` for a full scan or `--base origin/main` to override the diff base. The command returns the worker's exit code and JSON report.

This image contains PHPStan/phpstan-drupal/deprecation rules, PHPCS/Drupal Coder/PHPCompatibility, debug, Drupal config, syntax, whitespace, and YAML checkers, PHP 8.4.26, Git, and the locked Symfony YAML tool bundle. It requires no host PHP, Composer, or Go, and no project `vendor` directory. The image defaults to UID/GID 10001; the command above uses your UID/GID to read private project directories. Check mode mounts the project read-only and uses a private temporary filesystem. Git trusts only the known `/workspace` mount through the image's system configuration.

Use a local Docker daemon with access to the source directory. Mount the entire Git repository for monorepos and pass `--project /workspace/path/to/project`. Linked worktrees or separate Git directories whose metadata lives outside the mount need additional path mapping; the launcher will handle those separately. Container paths in findings remain relative to the configured project.

The tag is local and unpublished. Base images are pinned by digest, while Alpine packages still come from their package repositories. Release package locks, platform/framework compatibility entries, published digests, and automatic pulls remain future work. The current image is tested on Linux amd64 and does not establish a Drupal version support matrix.

## Repository checks

Native repository tests require Go 1.26.5, PHP 8.4 with tokenizer, mbstring, SimpleXML and XMLWriter, Composer 2.10.3, Python 3.12, and Git. Composer installs only the repository's isolated test tool dependencies. From the repository root:

```sh
python3 -m venv .venv
composer install --working-dir tools/yaml --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction
composer install --working-dir tools/phpcs --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction
composer install --working-dir tools/phpstan --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction
.venv/bin/python -m pip install -r requirements-dev.txt
.venv/bin/python -m ruff check tests
.venv/bin/python -m ruff format --check tests
.venv/bin/python -m unittest discover -s tests -v
go vet ./...
go test -race ./...
php -l internal/checker/debug/helper.php
php -l internal/checker/yaml/helper.php
php -l internal/checker/phpcs/helper.php
php -l internal/checker/phpstan/helper.php
.venv/bin/python -m unittest tests.acceptance.worker_cli -v
```

Go tests exercise configuration validation, defaults and overrides, project selection in monorepos and Git worktrees, and the actual compiled CLI's JSON output and exit codes. Scope tests also materialize the existing patch fixtures and verify file/line selection, unusual filenames, deletion/rename behavior, exclusions, and merge-base semantics. The JSON schema is embedded in the binary; configuration validation needs no network access or Docker.

Python fixture tests validate the scenario data and Git states. The separate worker acceptance suite builds the real CLI, runs all six fixtures in four scope modes with the real PHP helper, and compares semantic findings and exit codes. It also injects process failures, checks optional policies, and tests configuration overrides.

The Docker suite runs the same six fixtures in four modes against the real image, checks read-only mounts and the default non-root user, verifies operation without host PHP/Go, and covers concurrent containers, runtime mismatch, and monorepo paths. Run it explicitly after building the image; it requires Python, Git, and Docker, without host PHP or Go:

```sh
PHPWARDEN_TEST_IMAGE=phpwarden:static-php8.4-dev \
  python3 -m unittest tests.acceptance.docker_worker -v
python3 -m unittest tests.acceptance.launcher_cli -v
```

The launcher suite extracts the actual CLI binary from the built image and runs public `check` across the same 24 fixture combinations with no PHP or Go on the client PATH. It also checks linked worktrees, monorepos, offline resolve, and settings overrides. Go tests inject missing Docker/images, invalid reports, changed inputs, cancellation, and timeout failures.

Five syntax fixtures add 20 combinations per worker, Docker, and launcher suite: added parser errors, old errors outside diff hunks, deletion-only errors at EOF, compile errors, and untracked paths with spaces. Shared acceptance checks cover explicit checker selection, disabled defaults, nonblocking findings, extra extensions, exclusions, and source that must never execute. Optional execution failures keep `complete: false`; already observed blocking findings still return exit 1 unless a required execution failure takes precedence with exit 3.

Six YAML fixtures add another 24 combinations per suite: duplicate keys, old errors outside hunks, deletion-only errors, uppercase untracked paths with spaces, valid aliases/custom tags, and invalid UTF-8 without a line number. Settings tests cover include/exclude, disabled/explicit selection, empty files, nonblocking findings, and project autoload isolation. Native worker tests also remove the adjacent tool bundle to verify required and optional failure policies. The native acceptance harness creates a temporary binary-and-library layout; Docker tests use the actual bundled installation with no network at check time.

Six whitespace fixtures add 24 combinations per suite: added spaces/tabs, old violations outside hunks, deletion-only changes, Markdown conflicts, untracked CRLF/EOF content with a spaced filename and binary data, and marker lookalikes. Shared settings tests cover independent rule toggles/exclusions, explicit selection, include/exclude, and nonblocking findings. Go tests also cover long lines, invalid UTF-8 text, late NUL bytes, cancellation, and partial read failures. Intentional fixture whitespace is exempted from repository `git diff --check` via a scoped attribute.

Eight Drupal config fixtures add 32 combinations per suite: new keys, old keys outside hunks, deletion-only changes, quoted/flow YAML, inherited keys, clean nested schema keys, files outside module config directories, and an explicit custom module root. Shared settings tests cover only/disabled, custom keys and directories, include/exclude, nonblocking findings, malformed YAML required/optional policies, custom/standalone module roots, site-export exclusion, and autoload isolation. Go tests also cover escaped keys, BOM/CRLF, merge precedence/positions, invalid aliases, nesting limits, cancellation, and partial read failures.

Eight PHPCS fixtures add 32 combinations per suite: Drupal style issues, old issues outside diff, deletion-only changes, untracked newline/spaced paths, clean source that must not execute, PHPCompatibility target checks, project ruleset priority, and nearest ruleset grouping. Shared settings tests exercise disabled/only, severity/exclusions, warning policy, optional configuration failures, compatibility overrides, and a real project custom sniff without loading the project Composer autoloader. Request tests reject changed/new/transitive rulesets and changed/new custom standard files. Older checker fixture configurations explicitly disable PHPCS and PHPStan to keep their expected findings focused on their own rules.

Seven PHPStan fixtures add 28 combinations per suite: added type errors, old errors outside diff, deletion-only changes, untracked newline/spaced paths, added deprecated calls, clean source that must not execute, and project config priority. Shared settings tests exercise disabled/only, level, exclusions, fail policy, required/optional configuration failures, missing Drupal dependencies, baselines, and the deprecation switch. Project config tests cover discovery order, `discover_config: false`, explicit levels, invalid NEON, and `vendor-dir` autoload discovery. Request tests reject changed/new/transitive configs and changed/new dependency files. Go tests also reject malformed, inconsistent, or crashing PHPStan output and verify that agent variables do not reach PHPStan.

Drupal discovery acceptance covers root/web/docroot/custom web roots, multisite custom directories, automatic source roots, explicit broad roots with profile exclusions, locked version reporting, malformed metadata, and version conflicts. These are small synthetic Git projects; they do not establish compatibility with a full installed Drupal application.

Composer compatibility tests cover production/dev distinctions, target gaps, intersecting patch restrictions, simulated platform settings, optional vendor, Composer 1/2 installed metadata, custom vendor paths, no-dev layouts, metadata changes, and actual worker PHP patch verification. Docker acceptance exercises matching/mismatching installed Drupal versions and image patch incompatibility without executing project autoload.

GitHub Actions checks Go formatting, vet, race tests, a standalone build, PHP helper syntax, and worker acceptance alongside Ruff and Python fixture validation. A separate Docker job builds the Linux amd64 development image and runs both Docker and public launcher suites without installing host Go/PHP. The worker test environment uses PHP 8.4. Image publishing and a version/platform matrix are not configured yet.
