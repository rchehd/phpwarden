# Drupal preparation for Psalm

This candidate bridges PHPWarden's existing Drupal discovery facts to the pinned
upstream Psalm plugins. It is exercised by the development acceptance harness,
not registered in `check`, distributed in workers or exposed in project settings.

`prepare.php REQUEST.json EMPTY_OUTPUT_DIRECTORY` runs with the isolated tool
vendor mounted at `/tool-vendor`. The request contains `root` (absolute repository
root), `composer_root`, `web_root`, `source_roots`, `include` and `exclude` (paths
relative to the repository). The acceptance harness obtains these from the real
`phpwarden config show --format=json` response. Discovery, monorepo and Composer
path-repository rules remain owned by the existing Go Drupal profile.

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
is `tools/psalm-spike/models/psalm.php`; its overlays retain upstream annotations
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
