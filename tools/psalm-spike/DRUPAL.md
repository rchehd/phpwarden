# Upstream Drupal plugin compatibility

Evaluated on 2026-10-29 with PHP 8.3, installed locked Drupal 10.6.0 and 11.4.1 fixtures. The source projects were copied, never mounted into analysis containers. The copies contained core and installed dependencies, without site settings, a database or an application server. This is a development experiment, not an enabled PHPWarden checker.

## Reproduce

The [tool manifest and lock](upstream/composer.json) pin Psalm 6.20.3, `mortenson/psalm-plugin-drupal` at `073e7a4d5fc159fbf47decf16296edd7f8a545d1`, and Symfony plugin 5.3.0. The isolated bundle resolves Symfony runtime libraries to 7.4. Its runtime and the application's Symfony version are distinct; native plugin stubs select the application's major version.

Install the tool bundle into a disposable directory using PHP 8.3 and Composer. Installation may access the network; analysis does not. This example uses an existing local dependency image, the same as the initial spike:

```sh
mkdir -p /tmp/phpwarden-psalm-tool-vendor
docker run --pull never --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$PWD/tools/psalm-spike/upstream,dst=/bundle,readonly" \
  --mount type=bind,src=/tmp/phpwarden-psalm-tool-vendor,dst=/tool-vendor \
  --env COMPOSER_HOME=/tmp/composer-home --env COMPOSER_VENDOR_DIR=/tool-vendor \
  --workdir /bundle --entrypoint /usr/bin/php8.3 ddev/ddev-webserver:v1.25.5 \
  /usr/local/bin/composer install --no-scripts --no-plugins --no-interaction --prefer-dist --no-progress
python3 tools/psalm-spike/drupal.py \
  --project /path/to/installed-locked-drupal-fixture \
  --bundle /tmp/phpwarden-psalm-tool-vendor \
  --image ddev/ddev-webserver:v1.25.5 --php /usr/bin/php8.3 \
  --output /tmp/phpwarden-psalm-drupal-new-run
```

The input must have the `web/` fixture layout and installed dependencies. The harness copies Composer metadata, vendor, core and the scaffold autoloader, then adds its own module. It never installs into the input project, runs HTTP requests, processes a queue, builds images or boots a database. The module contains deliberately unsafe examples; do not install it in a working application.

Output includes native stdout/stderr, per-phase config/entrypoint/helper snapshots under `inputs/`, and `summary.json`. Exit 1 means the candidate did not satisfy the controls. The profile-model experiment passes those controls on both tested versions. `taint_only_json` only describes native output shape. It does not mean complete or successful analysis. `integration_ready` remains false: passing a fixture suite alone does not register a checker or complete the integration work. Each phase uses a uniquely named owned container, cleaned up even on exceptions or interruption.

## Results

The evaluator removes its private project copy after the run, including failures, while retaining inputs and reports. Use `--keep-workspace` only when the copied project is needed for debugging. The source fixture and supplied tool bundle remain unchanged. An active container mount or failed resource inspection prevents deletion and fails cleanup.

Both Drupal versions produced the same coverage results with the custom module autoloader. Findings and taint traces are retained without suppression, baseline changes or lowered gates.

| Control | Module autoloader | Fixture renderer metadata | Explicit route/queue inputs | Profile models + explicit inputs |
| --- | --- | --- | --- | --- |
| Request helper → trusted markup | Found | Found | Found | Found |
| Request helper → concatenated SQL | Found | Found | Found | Found |
| FormState input → trusted markup | Found | Found | Found | Found |
| Preprocess hook → trusted markup | Found | Found | Found | Found |
| Request helper → inline template | Missed | Found | Found | Found |
| Required route argument → direct HTML output | Missed | Missed | Found | Found |
| Queue payload → concatenated SQL | Missed | Missed | Missed | Found |
| `Html::escape`, `#plain_text`, filtered plain-string `#markup`, SQL placeholders | No findings | No findings | No findings | No findings |
| Queue payload → bound SQL parameters | No findings | No findings | No findings | No findings |

The last two columns use explicit synthetic call sites for the fixture's known route and queue inputs. The SQL stub's unspecialized sink merges call sites: the shorter controller flow appears while the queue path disappears. An isolated queue-only scan finds the original unsafe flow; changing inheritance, parameter annotations or method specialization does not restore it in the combined scan. The experimental overlay specializes `Connection::query` sinks per call and retains the upstream SQL taint annotation. Both SQL paths then appear together; bound queries remain clean. This does not identify real queue producers or model persistence between requests.

The renderer metadata is a deliberately small XML service map for this fixture, not a complete application container dump. Without the renderer's type, an unsafe inline-template flow disappears. Production integration must track unresolved services and model coverage rather than claim a clean scan from an empty report.

## Setup and completeness failures

The upstream kernel autoloader is unsuitable unchanged for this standalone bundle:

- On Drupal 11 it tries to obtain a database connection during container compilation and fails without one.
- On Drupal 10, the separately installed Symfony 7 runtime conflicts with Drupal 10's kernel method signatures before compilation completes. This is the tested standalone-bundle combination; it does not establish that a project-native installation with compatible Symfony dependencies fails the same way.

The experiment's alternative bootstrap only loads the real project autoloader and registers the fixture module's namespace. It does not compile a Drupal container or instantiate controllers. This allows the unmodified upstream taint rules to run without a site or database.

The upstream route generator invokes callbacks with no arguments. The fixture's required route argument yields `TooFewArguments` during ordinary Psalm analysis. The explicit-call experiment supplies that argument from a known request source; this is not a general implementation of Drupal's argument resolver.

Ordinary Psalm reports that the generated `FormState` object does not implement `FormStateInterface`. The actual PHP runtime confirms that it does. The upstream source stub redeclares `FormState` without its interface. The experimental [overlay](models/form-state.php), registered after upstream stubs, restores the interface without replacing the annotated input methods. Ordinary analysis then exits 0 on both fixtures and form taint detection remains intact; no diagnostic is suppressed.

The [launcher](models/psalm.php) lazily loads only the overlay plugin before using Psalm's Composer entrypoint. Preloading the tool vendor eagerly can mix Symfony 7 tool classes with Symfony 6 application classes and fail while parsing service XML. This loading-order check is separate from the upstream kernel bootstrap incompatibility above.

A malformed helper provides a failure control: PHP lint exits 255 and ordinary Psalm reports `ParseError`, while taint-only mode still returns JSON with an unrelated remaining finding and no parser diagnostic. Missing Composer autoloaders fail before producing taint JSON. An adapter must independently check input readiness and parsing, preserve diagnostics, and mark partial results incomplete.

Original Composer/core fingerprints and copied probe source hashes remain unchanged after the experiments, including restoration of the intentionally broken helper and removed private autoloader. Site settings remain absent. Compact evidence is archived locally under `docs/artifacts/psalm-models-2026-10-29`; temporary projects are removed after review.

## Decision

Reuse of the upstream rules is promising: the overlay restores seven unsafe behaviors while all five safe behaviors remain clean, and ordinary analysis has no setup diagnostics. The pinned package is not ready to be bundled unchanged into the normal check pipeline. Remaining work is a production database-free profile bootstrap, dependable service/entrypoint discovery and a readiness/completeness contract with failure fixtures. Fixture-specific route calls and renderer XML cannot serve as a general Drupal preset. Verify further PHP/runtime combinations and dependency/license distribution requirements separately.

No production checker, defaults, toolset revision, worker image, project hook or agent installation was changed by this experiment.
