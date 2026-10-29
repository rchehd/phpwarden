# Drupal preparation for Psalm

The internal worker pipeline bridges PHPWarden's Drupal discovery facts to the
pinned upstream Psalm plugins. Profile PHP assets live under
[`internal/profile/drupal/psalm`](../../../internal/profile/drupal/psalm) and are
embedded in the binary. The development harness supplies an isolated analyzer
bundle. Public `check` registration, project settings and image packaging remain
separate work.

`prepare.php REQUEST.json EMPTY_OUTPUT_DIRECTORY [TOOL_VENDOR]` defaults to the
isolated tool vendor at `/tool-vendor`. The request contains `root` (absolute repository
root), `composer_root`, `web_root`, `source_roots`, `include` and `exclude` (paths
relative to the repository). The acceptance harness obtains these from the real
`phpwarden config show --format=json` response. Discovery, monorepo and Composer
path-repository rules remain owned by the existing Go Drupal profile.
The worker also supplies `inventory` and `files`, computed with the core's path
selection rules. The returned PHP source list must match exactly; omitted files
cannot produce a complete result. `cache_directory` names the private run cache.

The helper reads selected `.info.yml` files to identify module/theme/profile
names and namespace mappings. It reads the core and selected extensions' service
YAML, extracting literal classes, simple class parameters and aliases. Symfony
YAML handles tagged argument values without constructing services. Symfony XML
metadata supplies the existing Drupal/Symfony plugins with these service types.
Conflicting definitions and decorators are unresolved rather than assigned an
arbitrary enabled-site precedence. Unused unresolved definitions are listed;
referenced unresolved services produce diagnostics.

PHP-Parser reads selected source files without executing them. Route controllers
can use `Class::method`, `service:method`, `service::method` or `Class` with
`__invoke`; class constructors are not called. Typed instance stubs provide
controller/form objects. Literal defaults, optional parameters, scalar route
placeholders and injected Request objects are supported. Route placeholders are
modeled as request input, with casts for native scalar parameter types. Typed
array results go through Drupal's renderer. Untyped render results, entity
conversion and unresolved signatures need further modeling and produce diagnostics.
Forms use upstream FormState sources. Hook bodies are analyzed as source; their
external callback arguments are not automatically modeled as tainted input.

All generated files stay in the private output directory: `bootstrap.php`,
`services.xml`, `instances.php`, `entrypoints.php`, `psalm.xml` and `manifest.json`.
The manifest records selected extensions, source files, route coverage, service
types, unresolved definitions, diagnostics and SHA-256 of consumed inputs.
`preparation_complete` describes only this metadata preparation;
`analysis_complete` remains false. Exit 3 indicates incomplete preparation or a
parse/input failure. The caller must not turn it into a clean analysis result.

Run Psalm with the generated config and the Composer project as `--root`, including
its repository-relative prefix in a monorepo. The current development launcher
is `internal/profile/drupal/psalm/psalm.php`; its overlays retain upstream annotations
while fixing FormState structure and specializing SQL sinks. Use a separate
process for preparation and analysis so dependency load order is not inherited.
The upstream plugin searches the current directory for module stubs, so the
harness uses the private output directory as cwd and lists source files explicitly.

The helper does not infer enabled modules or site configuration, compile a
container, boot Drupal or connect to a database. Custom ServiceProviders,
service imports, dynamic lookups, queue persistence and source symlinks are
reported as unsupported when encountered. Contributed-module service discovery,
arbitrary plugins and site-specific service overrides require further work.
The generated bootstrap loads the installed Composer autoloader during Psalm;
normal Composer autoload behavior still applies. Project and tool inputs are
mounted read-only and analysis runs without network in the acceptance harness.

## Internal worker pipeline

`worker.AnalyzePsalm` runs the framework-neutral `internal/psalmanalysis` pipeline
with Drupal-owned preparation and plugin pins. It verifies locked versus installed
project packages and the Composer autoloader, then checks PHP and required
extensions. This bundle is currently validated only on PHP 8.3.16+ in the 8.3
series, and the actual runtime must match the project's requested runtime.

Preparation must return complete metadata and a verified input inventory. The
worker syntax-checks source/generated PHP without executing it, requires ordinary
Psalm analysis to return no diagnostics, then runs taint analysis over the full
prepared graph. This readiness policy is deliberately conservative: existing
ordinary Psalm issues currently stop the taint phase. Native findings keep their
full JSON and traces. Invalid exits, partial reports, missing sources, malformed
trace locations, process failures, cancellation or changed project/prepared inputs
cannot produce `complete: true`. Private files are removed on every return path.

The default returned object is internal validation evidence. The development
worker also accepts `--format json` or `--format text` for a full-scan check report,
with `--all` expanding text traces. Psalm remains outside public `check` until
project settings, native config/baseline overrides and tool/image packaging pass
their integration gates.

`internal/checker/psalm.Normalize` converts verified results to Security findings.
The optional report v1 `trace` field keeps every ordered native label, snippet and
available line/column range. Each step identifies a `project`, `tool`, `generated`
or locationless `symbol` origin; paths are relative to that origin's root.
The internal native evidence retains the original JSON, including byte offsets.
Ordinary project sinks stay at their native location. A sink in generated code
is anchored to the last analyzed project step, while its generated endpoint
remains in the trace. A flow without an analyzed project anchor is incomplete.
Invalid findings abort normalization even if they would be outside report scope.

Analysis still uses the full prepared graph. Report selection includes a flow
when any project file in its trace or its primary location is selected. This is
whole-file selection even in diff mode: native steps can point at a method's
signature rather than the changed statement. Changing consumed route, service,
extension, Composer or project configuration metadata conservatively selects
all flows. Callers must include those context files in the shared scope; unrelated
metadata does not broaden the report. Empty or unrelated scope returns no flows.
Fingerprints include the normalized flow, independent of temporary root names
and source snippets. Distinct flows at the same sink survive deduplication.
The development report uses `required: true`, `fail_on: never`; findings remain
advisory and execution failures still make analysis incomplete.

Default text shows at most six trace steps per displayed finding, with an explicit
omission count. It shortens class names, reuses the primary file's heading for
same-file locations and omits tool/generated file locations. `--all` shows every
step with full labels and locations. JSON always includes the complete trace.
The existing compact format keeps its one-line finding presentation.

## Acceptance

Install the committed Drupal 10/11 sandbox locks and
[`tools/psalm-spike/upstream`](../../psalm-spike/upstream) tool lock into disposable
directories, then supply a locally built PHPWarden binary and existing PHP image:

```sh
python3 tools/psalm-spike/discovery.py \
  --project /scratch/installed-drupal10 \
  --bundle /scratch/psalm-vendor --binary bin/phpwarden \
  --image ddev/ddev-webserver:v1.25.5 --php /usr/bin/php8.3 \
  --output /tmp/phpwarden-psalm-discovery-d10
```

For the complete internal worker path, build its development entrypoint with
`GOMAXPROCS=1 go build -p=1 -o /scratch/psalm-worker ./tools/psalm-spike/worker`
and add `--worker /scratch/psalm-worker` to the command above. The extended suite
checks that missing autoloaders, unresolved classes and PHP syntax unsupported by
the selected runtime stop before the taint phase. Containers run sequentially
with a default `--cpus 0.2` quota; increase it explicitly only when resources permit.

Repeat with the Drupal 11 fixture and a new output directory. The harness copies
only core, vendor, Composer metadata and the scaffold autoloader, then adds its
own renamed module with constructor dependencies. It tests real Go discovery,
service aliases and parameters, routes/forms, full native readiness and taint
controls, nested Composer roots and a module outside webroot. Negative cases
cover dynamic/missing services, alias cycles, malformed PHP/YAML, entity
conversion, queue entrypoints and source symlinks. Includes/excludes are checked
through the actual project configuration. The private project is removed on
success/failure; `--keep-workspace` retains it explicitly for debugging. Reports
and prepared metadata remain under the output directory.
