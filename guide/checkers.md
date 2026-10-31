# Checkers

## Checkers

All checkers except `composer-require-checker`, `composer-unused` and `psalm` are enabled by default in the Drupal profile. `composer-audit` is the only one that uses the network, and it is optional by default.

These checks require an explicit choice for the project:

| Checker | Why it starts disabled |
|---|---|
| `composer-require-checker`, `composer-unused` | Inspect a whole standalone Composer package in full mode; Drupal installer layouts are unsupported |
| `psalm` | Experimental taint analysis; requires a compatible worker bundle, currently validated on PHP 8.3 |

Enable one with `tools.<id>.enabled: true`, or select it for a single run using `--only`. `disabled` means configuration turned a checker off; `no files` means it has no applicable files in the selected scope; `no config` means a required native rules file was not found. Checkers use that scope and their documented reporting policy; prerequisites still apply.

| ID | Checks |
|---|---|
| `composer-validate` | `composer.json` and `composer.lock` validity, like `composer validate` |
| `composer-audit` | Known vulnerabilities and abandoned packages in `composer.lock`, like `composer audit` |
| `dead-code` | Unused methods, properties, constants, and enum cases; enabled, advisory findings |
| `debug` | Debug leftovers: PHP calls such as `var_dump`, `print_r` (not with `$return`), Devel's `dpm`/`kint`/`dd`, and `die`; `console.log` and `debugger` in JavaScript (not `*.min.js`); `dump()` and `kint()` in Twig |
| `syntax` | PHP syntax (`php -l`, optional Parallel Lint) in `.php`, `.module`, `.inc`, `.install`, `.theme`, `.profile`, `.engine`, `.test` |
| `yaml` | YAML syntax |
| `whitespace` | Trailing whitespace and unresolved merge conflict markers |
| `drupal-config` | Top-level `uuid` and `_core` keys in module `config/install`, `config/optional`, `config/schema` |
| `eslint` | JavaScript lint using the nearest project config; skipped without a config |
| `eventhorizon` | Drupal caching heuristics and exported config relationships; enabled, advisory |
| `gitleaks` | Potential secrets in selected files; values and source text are redacted |
| `stylelint` | CSS/SCSS/Sass/Less lint using the nearest project config; skipped without a config |
| `jscpd` | Duplicate code across custom source roots, reported as warnings |
| `phpcs` | PHP_CodeSniffer with Drupal Coder (`Drupal`, `DrupalPractice`); PHPCompatibility on request |
| `phpmd` | PHPMD: unused code and code size, reported as warnings |
| `phpstan` | PHPStan with phpstan-drupal and deprecation rules |
| `psalm` | Opt-in taint flows across custom PHP files; advisory, requires a Psalm-capable worker |
| `composer-require-checker` | Undeclared symbols in standalone Composer packages; opt-in, full only |
| `composer-unused` | Dependency removal candidates; opt-in, advisory, full only |
| `deptrac` | Project-defined architecture layers and dependency rules; skips without config |
| `rector` | Drupal/PHP modernization suggestions from an isolated Drupal Rector bundle; enabled, dry-run |
| `semgrep` | Local PHP security search/taint rules using Semgrep CE; enabled in the Drupal profile |
| `twig-cs-fixer` | Twig template style with Twig CS Fixer |

Every checker accepts these settings under `tools.<id>`:

- `enabled`: run the checker.
- `required`: when the checker cannot run, `true` fails the whole check, `false` reports an incomplete result.
- `fail_on`: `error`, `warning`, or `never` (report without failing).
- `timeout_seconds`, `include`, `exclude`.

### PHPCS

PHPCS uses the nearest `.phpcs.xml`, `phpcs.xml`, `.phpcs.xml.dist`, or `phpcs.xml.dist` in the project. Without one it uses `Drupal` and `DrupalPractice`. Other useful settings are `standards`, `sniffs`, `exclude_sniffs`, `standard_paths` for custom standards, and `compatibility.enabled: true` for PHPCompatibility against `php.target`.

### PHPMD

PHPMD reports unused code and oversized classes and methods as warnings, so it does not fail the check; set `fail_on: warning` to block on them. It is optional by default, so an execution failure makes the run incomplete without failing it; set `required: true` to fail the run. A finding about a method or class shows when its declaration line changes.

PHPMD uses the nearest `phpmd.xml`, `phpmd.xml.dist`, or `.phpmd.xml` in the project, and a `phpmd.baseline.xml` next to it. Paths in the ruleset's `<rule ref>` are relative to the project root. Without a project ruleset it uses `rulesets` (`unusedcode` and `codesize`) without `exclude_rules` (`UnusedFormalParameter`, because Drupal hooks have fixed signatures). Other settings are `config`, `discover_config`, `extra_extensions`, `extra_inputs`, and `memory_limit_mb`. A file PHPMD cannot parse is reported as a warning for the whole file.

### Dead code

`dead-code` is enabled by default in the Drupal profile; run it alone with `phpwarden check --only dead-code` or disable it with `tools.dead-code.enabled: false`. An explicit disable in an existing project config takes precedence over the profile default. It uses PHPStan with [dead-code-detector](https://github.com/shipmonk-rnd/dead-code-detector), so it needs the same installed Composer dependencies and PHPStan settings. A member is dead only if nothing in the project uses it, so it analyses all custom code and reports only members declared in the checked files and lines. Findings are warnings; set `fail_on: warning` to block on them. The checker is optional by default, so an execution failure makes the run incomplete without failing it; set `required: true` to fail the run.

Drupal calls much of its code without a PHP call site, so these count as used: constructors, members overriding code outside the source roots such as Drupal core or contrib, methods with Drupal hook or Drush attributes, `'::method'` and `[$this, 'method']` callbacks in the same class, any method name used as a string in event subscribers and trusted-callback classes, and `Class::method` or `service:method` in the module's routing and services YAML. Other entry points, such as callbacks in another module, can still show up as false positives. Settings are `timeout_seconds` (600) and `memory_limit_mb` (1024).

### ESLint and Stylelint

Node.js is included in the worker image; no host Node.js is needed. Each file uses its nearest project config, falling back to `composer_root` for code outside the Composer project. `config` explicitly selects a project-relative config; `discover_config: false` disables discovery. Without a config the checker reports `skipped: no_config`, with a diagnostic identifying the affected files. No rules are invented for an unconfigured project.

ESLint discovers `eslint.config.js`/`.mjs`/`.cjs`, `.eslintrc` variants, and `eslintConfig` in `package.json`. Stylelint discovers `stylelint.config.js`/`.mjs`/`.cjs`, `.stylelintrc` variants, and `stylelint` in `package.json`. ESLint 8–10 and Stylelint 14–17 project installations take precedence over the bundled ESLint 9.39.5 and Stylelint 17.16.0. ESLint 10 requires flat config. Declared but missing tool dependencies produce `unavailable`; install the project's packages before checking it. Plugins, shared configs, parsers, and SCSS syntax packages must be installed in the project and compatible with the chosen tool.

Rule findings respect changed lines and native severity. Syntax errors apply to the entire selected file. Fixing and native result caches are disabled. ESLint selects `.js`, `.mjs`, `.cjs`, `.jsx`; Stylelint selects `.css`, `.scss`, `.sass`, `.less`. Both support `extra_extensions` and `extra_inputs`; minified files are excluded by default. Imported configs and installed packages participate in stale-input detection.

### Duplicate code

`jscpd` uses the pinned Node engine 4.3.0. It searches all configured custom source roots, applying scope and checker exclusions, without following symlinks. Both sides of a duplicate may be outside the changed files, but a finding is shown only where a duplicate block overlaps selected lines. The reported line is the first selected line inside that block. Findings are warnings; use `fail_on: warning` to block on them.

Defaults are `min_tokens: 70`, `min_lines: 6`, `mode: mild`, a 300-second timeout, and `required: false`. These settings override native thresholds and mode. An explicit `config` or a root/Composer-root `.jscpd.json` can supply formats, extension mappings, ignores, and size limits. Native output paths, reporters, and global percentage thresholds do not control PHPWarden's report or verdict. PHP files with Drupal extensions such as `.module` and `.inc` use the PHP tokenizer. The scan includes eligible files beneath source roots, including files ignored by Git; exclude generated directories with `scope.exclude` or `tools.jscpd.exclude`.

### PHPStan

PHPStan loads the project's Composer autoloader, so run `composer install` first. The installed `drupal/core` must match `composer.lock`. If it does not, PHPStan is reported as unavailable and the other checkers still run.

PHPStan uses `phpstan.neon`, `phpstan.neon.dist`, or `phpstan.dist.neon` from the project root. Without one it analyses at level 3. Other useful settings are `level`, `baseline`, `includes`, `bootstrap_files`, `deprecations`, and `memory_limit_mb`.

#### Strict rules

The bundled `phpstan/phpstan-strict-rules` 2.1.0 is opt-in through `tools.phpstan.strict_rules.enabled: true`. Findings use the normal PHPStan severity, scope, baseline, and failure policy; there is no separate checker ID. `strict_rules.all` defaults to `true`; `strict_rules.rules` overrides individual native switches, such as `disallowedLooseComparison`, `disallowedEmpty`, `booleansInConditions`, and `strictFunctionCalls`. Use `all: false` with selected switches set to `true` to choose rules individually. The schema lists all 19 switches from the pinned package.

Loading this extension also changes core PHPStan parameters. `enabled: true, all: false` keeps those changes even with no named rules selected. `enabled: false` means PHPWarden does not include the package; a native project NEON may still include it explicitly. PHPWarden's strict-rule switches apply only to the normal PHPStan run, not to the separate `dead-code` run. Advanced parameters remain in project NEON or `tools.phpstan.includes`.

### EventHorizon

`eventhorizon` is enabled by default in the Drupal profile, with advisory findings (`fail_on: never`). Disable it with `tools.eventhorizon.enabled: false`, or run it on its own with `phpwarden check --only eventhorizon`. An explicit `enabled: false` in an existing project config retains precedence over the profile default.

It uses the caching and config APIs from [QED42 EventHorizon](https://github.com/qed42/eventhorizon-cli-mode), pinned to 0.1.0 commit `ae11df1cd8e4e2b843c6c53bac60026ef344935f`, with PyYAML 6.0.3. Python and the sources are bundled in the worker; Drupal bootstrap, Composer dependencies, and a site database are not required. The integration includes caching detectors and config relationship checks. General regex security rules and code metrics are outside this adapter.

Caching checks use whole selected PHP/Drupal files, then filter findings to selected lines. Exported config requires explicit project-relative `config_sync` directories. Include those directories in `scope.include` as well as your source roots. When a selected `.yml` file belongs to one of them, all top-level `.yml` files in that directory are analyzed and fingerprinted, including files outside the diff and checker include/exclude filters. Relationship findings describe the directory as a whole and remain visible in a deletion-only diff. Config sync paths are not discovered by executing `settings.php`.

Defaults are `caching: true`, `config_sync: []`, `exclude_rules: []`, `timeout_seconds: 120`, `required: false`, and `fail_on: never`. Findings are warnings or info because these are heuristics; a missing cache marker can be intentional or supplied elsewhere. Use `exclude_rules` for known exceptions, such as `caching.missing_cache_metadata` or `config.orphaned_entity`; the schema lists supported rule IDs. `fail_on: warning` enables blocking. A broken analyzer makes the result explicitly incomplete by default; `required: true` turns that into an error.

Analysis uses private copies of inputs. Symlinks, unreadable files, invalid text, malformed/duplicate-key/non-mapping config YAML, analyzer warnings/errors, unexpected output, and inputs totaling over 32 MiB are rejected. No source files or CSV/XLSX reports are written to the project. Accuracy and performance still need validation on real Drupal projects before considering wider use.

### Composer dependency checks

`composer-require-checker` and `composer-unused` are disabled by default. Enable them explicitly, or use `--only composer-require-checker,composer-unused --mode full`. Other modes report `skipped: full_scan_required`. They inspect the whole selected Composer package, including dependencies, regardless of diff or source include patterns. Both default to advisory `fail_on: never`; use `warning` to block on candidates. Neither removes packages or supports `--fix`.

Set each tool's `composer_root` to choose a standalone package; otherwise the global Composer root applies. Composer 2 installed metadata and all direct dependencies must exist; missing dependencies produce `unavailable: dependencies_missing`. The supported layout is `<vendor-dir>/<vendor>/<package>` with regular local files. Drupal installer paths such as `web/core`, virtual/replaced packages and linked dependencies report `skipped: unsupported_layout`. No blanket Drupal exclusions are supplied. In particular, YAML, plugin discovery and dynamically referenced services can make composer-unused candidates false positives: review them before removal.

ComposerRequireChecker is pinned to 4.7.1 on PHP 8.1 and 4.16.1 on PHP 8.2–8.4. Configure native JSON through `config`/`discover_config`, `scan_files` relative to the package root, and `allowed_symbols`. Native JSON accepts `scan-files`, `symbol-whitelist` and `php-core-extensions`. Root autoload sources and direct dependency symbols are analyzed; `autoload-dev` is not included automatically. `composer-unused` 0.9.7 supports native PHP `config`, `exclude_packages` and package-relative `exclude_directories`. Config discovery checks the project root, then the selected Composer root (`composer-require-checker.json` or `composer-unused.php`). PHP config and Composer's `installed.php` are trusted executable inputs inside the worker. List additional config inputs in `extra_inputs`. Both tools support timeout/memory limits and reject incomplete native reports.

### Parallel syntax checking

`tools.syntax.backend: parallel-lint` selects the isolated PHP Parallel Lint 1.4.0 backend; `parallel_jobs` defaults to 4 (range 1–32). It lints exactly the selected Drupal PHP files, without executing source/autoload. Native lint skip directives produce an incomplete result, never a clean pass. File names containing semicolons or line breaks require the default `php` backend. Parallel Lint is optional; no performance advantage has been established. The check ID and rule remain `syntax` / `php.syntax`.

### Deptrac

`deptrac` checks the architecture defined by your project and is enabled by default. Without a discovered configuration it reports `skipped: no_config`, including with `--only`, with guidance for installing and using the architecture skill. A missing explicitly configured file is an error. PHPWarden does not supply default Drupal layers; the optional `phpwarden-architecture` agent skill helps create project-specific rules. An existing `enabled: false` remains effective.

The isolated bundle pins Deptrac 4.4.0 and PHPStan 2.3.1. Version 4.4.0 is the newest Deptrac release compatible with the worker's complete PHP 8.1–8.4 matrix; Deptrac 4.5 and newer require PHP 8.2. All bundle dependencies have a separate lock and manifest identities. The adapter uses the pinned analysis API, disables Deptrac's persistent cache, and keeps generated configuration and temporary files in a private directory. It avoids the CLI analysis command's unlimited-memory override and retains `memory_limit_mb`.

Discovery checks `deptrac.yaml`, `deptrac.yml`, `deptrac.yaml.dist`, and `deptrac.yml.dist`, first at the project root and then at the Composer root. `config` sets an explicit project-relative YAML path. Configure `paths`, `layers`/collectors, `ruleset`, `exclude_files`, `skip_violations`, and analyzer settings under the native `deptrac` key. Graph paths are directories relative to the main native config. Local YAML imports, including baseline files, are supported as strings or `{resource: file.yaml}` entries. Composer collector `composerPath`/`composerLockPath` values are relative to the project working directory.

This adapter accepts self-contained YAML settings and explicit local YAML imports. PHP configs, custom services/parameters/tags, YAML anchors/aliases, remote/glob/optional imports, the experimental `phpstan_parser` flag, and graph paths containing symlinks or unreadable directories are unsupported. Inputs must remain inside the project. `extra_inputs` validates additional local files; project fingerprints also track graph/config/dependency metadata. No project PHP autoloader is explicitly loaded by the adapter.

Deptrac analyzes the complete graph from native `paths`, including files outside the diff. Its file collector scans `.php` files; Drupal classes in `src/` are covered, while procedural `.module`/`.inc` files are not. PHPWarden's `include`/`exclude` filters only findings, preserving graph context. Diff results require the dependency's source line to be selected. Changes to a dependency or architecture rules can affect unchanged callers: use `--mode full` for that review. A config-only diff without selected PHP files is skipped as `no_files`.

Forbidden dependencies are `deptrac.violation` errors. `report_uncovered` exposes `deptrac.uncovered` warnings; `fail_on_uncovered` always exposes them as errors. With `report_skipped`, accepted baseline entries are informational `deptrac.skipped` findings. Ambiguous layer assignments produce global `deptrac.configuration_warning` warnings. `fail_on` controls whether findings block the run; the default is `error`, and `never` makes findings advisory. Parsing errors, invalid rules, stale baseline errors, timeouts, and incomplete reports remain incomplete results regardless of this policy. Native and Docker fixture tests cover this behavior.

### Drupal Rector

`rector` is enabled by default in the Drupal profile and only suggests changes. It uses isolated, locked bundles of Drupal Rector 1.1.3, Rector 2.6.1, and PHPStan 2.3.1. Drupal Rector 1.1.3 explicitly conflicts with Rector 2.6.2 and newer; the lock preserves that constraint. Disable it with `tools.rector.enabled: false`, or run it on its own with `phpwarden check --only rector`. Existing project files with explicit `enabled: false` keep that choice.

With no project config or explicit targets, sets, or rules, the Drupal profile selects the bundled upstream deprecation sets through the current Drupal minor from resolved `framework.version`. For example, Drupal 11.4.10 uses sets through 11.4 and retains 11.4.10 for upstream version guards. Automatic presets support Drupal 10.0–10.6, 11.0–11.4, and 12.0; unknown or newer versions require a project config or an explicit selection. Presets suggest modernization for the current version without selecting a future Drupal upgrade.

An explicit `config` takes precedence over discovered `rector.php` / `rector.dist.php`; discovery checks the project root before the Composer root. Any project config, `target_php`, `target_drupal`, `sets`, or `rules` replaces the automatic preset. Explicit selections can be combined with a project config. `skip_rules` also works with the automatic preset. Upgrade targets are independent of the current project's `php.runtime`, `php.target`, and `framework.version`; they never change image selection or the normal compatibility checks. Without an explicit PHP upgrade target, Rector uses the current PHP target maximum as its feature ceiling and adds no PHP upgrade sets.

The automatic Drupal preset needs Composer-installed package metadata in the selected `composer_root` (including a custom `vendor-dir`). Restore project dependencies with `composer install` before checking. Missing metadata reports `unavailable: dependencies_missing`; PHPWarden does not install dependencies.

With applicable PHP files, an unresolved or unsupported automatic preset produces `error: config_invalid`. With its default `required: false`, this makes the run incomplete with exit code 0; `required: true` gives exit code 3. With no applicable files, it reports `skipped: no_files`.

`target_php` enables cumulative PHP sets through the chosen version (8.1–8.5). `target_drupal` selects the pinned minor deprecation sets through the chosen target, including older Drupal sets. Supported targets are 10.0–10.6, 11.0–11.4, and 12.0. This sets Drupal Rector's assumed destination and minimum supported version; suggested code may drop compatibility with older Drupal. The tool's rule coverage is incomplete and a successful check does not prove upgrade readiness. Future minor sets and the upstream opt-in breaking sets are not enabled automatically.

`sets` accepts bundled names such as `drupal-10.3` and `drupal-11.0`, or local PHP set files. `rules` and `skip_rules` accept fully qualified Rector class names. Configure rules needing options, custom rules/autoloading, extra sets, and native skipped paths inside project PHP config. `include`, `exclude`, and `extra_extensions` restrict selected files. `extra_inputs` validates additional local files; project context fingerprints also track metadata for imports, custom rules, and dependencies. Native PHP configs and project autoloaders execute project code, as they do for the other PHP analyzers.

The adapter always passes `--dry-run`, forces selected files and PHP extensions after importing project configuration, disables parallel processing, and puts caches in a private temporary directory. The worker mounts the project read-only. `dry_run` accepts only `true`; `--fix` continues to support PHPCS and Twig CS Fixer only. Suggestions identify affected original lines and the rules used for that file; raw patches are not exposed in the normalized report. Diff mode filters suggestions to selected changed lines; use `--whole-files` or `--mode full` for wider review. Native failures and partial reports produce an incomplete result, even when `fail_on: never`. Suggestions are warnings and advisory by default; `fail_on: warning` makes them fail the check. Native and Docker fixture tests cover this behavior.

### Semgrep CE

`semgrep` is enabled by default in the Drupal profile and runs with bundled rules without a project config. Disable it explicitly with `tools.semgrep.enabled: false`, or run it on its own with `phpwarden check --only semgrep`. Existing configurations that explicitly set `enabled: false` retain that choice, including files generated by older versions of `init`.

The worker includes the pinned Semgrep CE 1.182.0 engine. PHPWarden uses its native protocol directly; it does not install the Python CLI, contact the registry, send telemetry, or use Pro analysis. The bundled ruleset `1.0.0` contains four PHPWarden-authored taint rules for request data reaching shell commands, SQL query text, HTML/trusted markup, and `unserialize`. These rules cover `$_GET`, `$_POST`, `$_REQUEST`, `$_COOKIE`, and request `query`/`request` accessors. They do not treat every Drupal `#markup` value as an XSS vulnerability.

CE taint analysis operates within a function or top-level body. It does not track flows through other functions or files. PHPWarden analyzes whole selected files, including Drupal extensions, and reports matches intersecting selected lines. A newly changed taint source may feed a sink outside the diff; use `phpwarden check --only semgrep --whole-files` or a full scan when reviewing security changes. Findings indicate potential problems; they do not prove exploitability or complete security coverage.

Project rules are added through a root/Composer-root `.semgrep.yml` or `.semgrep.yaml`, an explicit `config`, and additional `rulesets` paths. `use_defaults: false` removes the bundled rules. These are self-contained local PHP search/taint YAML or JSON rules: remote configs, Jsonnet, validators, executable predicates, Pro features, autofixes, and native rule `paths` are unsupported. Use checker `include`/`exclude` for paths. `rules` selects IDs, `exclude_rules` removes IDs, and `severities` selects `ERROR`, `WARNING`, and `INFO`. Unknown/duplicate IDs are errors. CLI `nosemgrep` processing is not implemented by this adapter; use explicit rule exclusions or a baseline.

Defaults are `required: true`, `fail_on: error`, `timeout_seconds: 300`, `rule_timeout_seconds: 5`, and `memory_limit_mb: 1024`. Native ERROR/WARNING/INFO map to error/warning/info. Parse failures, skipped targets/rules, memory/time/fixpoint failures, or incomplete coverage make the checker incomplete; a successful process exit alone is insufficient. Inputs must be regular files without symlinks, at most 16 MiB each. Rules and baseline files are limited to 4 MiB each. Analysis runs on private copies and does not execute project PHP or write fixes. Rule text is used for messages; native source snippets, metavariable values, and traces are omitted from reports.

Semgrep findings expose `fingerprint` in JSON. To accept existing findings, first collect fingerprints from an unsuppressed report into a local JSON file shaped as `{"version":1,"fingerprints":["<64 hex characters>"]}`, then set `tools.semgrep.baseline` to its project-relative path. Fingerprints use the rule ID, original path, matched bytes, and duplicate occurrence, so unrelated line shifts preserve suppression. A baseline suppresses findings only; it cannot suppress analyzer failures. Native and Docker fixtures cover the pinned core and bundled rules.

### Psalm (integration in progress)

`tools.psalm` accepts `enabled`, `required`, `fail_on`, `timeout_seconds`,
`include`, `exclude`, `memory_limit_mb`, `config`, `discover_config` and `baseline`.
Defaults are disabled, required,
advisory (`fail_on: never`), 300 seconds and 1536 MiB. The supported candidate
runtime is PHP 8.3.16+ in the 8.3 series; another runtime is not selected implicitly.
An experimental PHP 8.4 probe found a property-hook taint false negative in the
pinned engine; [the regression](../tools/psalm/PHP84.md) keeps that runtime unsupported.
`init` exposes these settings and an optional checker selection; `--only psalm`
selects it for one check.

The current development images do not package Psalm yet. The launcher rejects
an applicable Psalm request when the selected toolset lacks that capability,
before starting a container. Worker integration can use the separately installed
pinned development bundle; image packaging, notices and the remaining runtime
matrix are separate gates. Existing workers
must be rebuilt together with this CLI because its checker/config contract changed.

The isolated bundle candidate and notice verification are in `tools/psalm`.
The pinned Drupal plugin's actual LICENSE includes Commons Clause in addition to
its Composer LGPL declaration; public image integration awaits a distribution
decision. See [bundle notices](../tools/psalm/NOTICE.md).

Native XML discovery checks `psalm.xml`, `psalm.xml.dist`, then `psalm.dist.xml`,
first at the repository root, then at `composer_root`. `config` chooses a file
explicitly; `discover_config: false` disables discovery. Supported XML settings
are `errorLevel` 1–6 (stricter readiness), `reportMixedIssues`, local `stubs` and
`issueHandlers` for `Tainted*` issues. Graph, plugins, runtime, autoloader and cache
remain profile-owned; unsupported native sections are rejected explicitly.

`baseline` selects a native Psalm taint baseline XML and overrides native
`errorBaseline`. Baseline `file src` paths are relative to `composer_root`;
`@generated/entrypoints.php` names the generated renderer/route sink. Copies are
rebased privately, leaving project files intact. Baselines and issue handlers apply
to the taint pass; the separate readiness pass ignores them. Syntax errors,
unresolved types or missing dependencies still make analysis incomplete. Supported
XML examples and native acceptance commands are in the development documentation.

Analysis uses the whole prepared graph within the configured source scope.
Per-checker `include`/`exclude` narrow reportable flows without pruning that graph.
A flow can be anchored outside those paths when it passes through an included
source or intermediate file. The current profile analyzes production custom code;
files under `tests` are inapplicable.
A changed source, intermediate or sink file selects the complete flow, even if
the changed lines differ from native trace locations. Consumed service, route,
extension or Composer metadata selects affected report scope conservatively.
JSON retains full traces; text shows a shorter chain, with `--all` for every detail.
Syntax/readiness failures remain execution diagnostics rather than clean security
results. Further model limits and reproduction steps are documented in
[`tools/psalm/drupal/README.md`](../tools/psalm/drupal/README.md).

### Gitleaks

Gitleaks 8.30.1 checks the current contents of selected files, including committed, staged, unstaged, and untracked changes. Diff mode reports only findings whose line ranges overlap selected lines; full and whole-files modes report the corresponding selected files in full. Multiline findings are anchored to the first selected line in the match. This scans files in the working tree, without scanning Git history or the index as a separate snapshot.

The checker is enabled and required by default. Findings are errors; `fail_on: never` makes them advisory. Secret values, matched text, native descriptions, and native process output are omitted from PHPWarden reports and diagnostics; native JSON reports are redacted and removed with the private temporary directory. Redaction is always enabled.

By default, it uses `.gitleaks.toml` and `.gitleaksignore` from the repository root, falling back to `composer_root`; otherwise it uses Gitleaks' built-in rules. `config`, `ignore_file`, and `baseline` accept explicit project-relative files. `discover_config: false` disables automatic config/ignore discovery. `rules` restricts enabled rules by their native IDs; custom rules and allowlists belong in TOML. Gitleaks resolves `[extend].path` relative to the repository root; list these transitive config files in `extra_inputs` so their changes invalidate a running check. Baselines are native JSON reports and suppress matches using Gitleaks' own matching semantics.

Gitleaks shares `scope.include` and `scope.exclude` with the other checkers. For example, a root `.env` is outside discovered Drupal custom roots; use `scope.include: ["."]` to include it. Checker `include`/`exclude` can further narrow the selected files. Archives and encoded payloads are not recursively scanned. No project files or baselines are generated or changed by the checker.

### Twig CS Fixer

Twig CS Fixer checks `.twig` templates. It uses `.twig-cs-fixer.php` or `.twig-cs-fixer.dist.php` from the project root. Without one it uses the default `TwigCsFixer` standard without `IncludeFunctionRule` and `CompactHashRule`, because Drupal templates use the `include` tag and long-form hashes. Other useful settings are `config`, `discover_config`, `extra_extensions`, and `extra_inputs` for files the project config loads.

Set `tools.twig-cs-fixer.autoload: auto` when a project config uses Drupal or other project token parsers, or set a relative autoloader path explicitly. This executes the trusted project Composer autoloader; it is disabled by default. The bundled Twig loader retains priority and its tool versions are recorded before project autoload. Changes to project autoload/config inputs invalidate the request context.

Twig CS Fixer checks style, not whether a template compiles. A template it cannot read at all is reported as `File.Invalid` even outside the changed lines.

### Composer validate

`composer-validate` checks the root `composer.json`, including whether `composer.lock` is up to date, when either file changed. In `--mode full` it always checks them. It also checks `composer.json` files of selected modules. Errors and an outdated lock are errors; other Composer warnings, such as exact version constraints, are warnings. Set `strict: true` to treat warnings as errors.

A root `composer.json` that is not valid JSON stops the check with exit code `2` before any checker runs, because PHPWarden reads the project layout from it.

### Composer audit

`composer-audit` checks the whole `composer.lock` on every run, whatever changed, because new advisories appear without code changes. Advisories come from the repositories in `composer.json`, such as Packagist and packages.drupal.org. Each advisory is a warning, so it does not fail the check; set `fail_on: warning` to block on them, for example in CI. Abandoned packages are reported as info; set `abandoned: ignore` to hide them.

Composer needs the network to fetch advisories. PHPWarden runs it in a separate container that sees only copies of `composer.json` and `composer.lock`; the other checkers stay offline. Results are cached in `~/.cache/phpwarden/advisories`:

- A cached result younger than `max_age_hours` (24) is used without the network.
- If fetching fails, a cached result younger than `max_stale_hours` (168) is used, and the report says so.
- Otherwise the check is unavailable, never clean. `offline: true` uses only the cache.

`environment.advisories` in the JSON report shows whether advisories came from the network or the cache, and when they were fetched. The check is optional by default, so working offline gives `PARTIAL` instead of a failure; set `required: true` in CI.

To accept a known advisory, list it with a reason and an optional end date:

```yaml
tools:
  composer-audit:
    ignore:
      - id: SA-CORE-2026-016
        reason: Patched in patches/core-xss.patch
        until: "2026-12-31"
```

`id` matches the advisory ID or its CVE. After `until`, the advisory is reported again.
