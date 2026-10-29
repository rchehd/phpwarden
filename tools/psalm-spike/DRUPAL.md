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

Output includes native stdout/stderr, per-phase config/entrypoint/helper snapshots under `inputs/`, and `summary.json`. Exit 1 means the candidate did not satisfy the controls; this is the observed result for both tested versions. `taint_only_json` only describes native output shape. It does not mean complete or successful analysis. `integration_ready` remains false: passing a fixture suite alone would not register a checker or complete the integration work. Each phase uses a uniquely named owned container, cleaned up even on exceptions or interruption.

## Results

Both Drupal versions produced the same coverage results with the custom module autoloader. Findings and taint traces are retained without suppression, baseline changes or lowered gates.

| Control | Module autoloader | Fixture renderer metadata | Explicit route/queue inputs |
| --- | --- | --- | --- |
| Request helper → trusted markup | Found | Found | Found |
| Request helper → concatenated SQL | Found | Found | Found |
| FormState input → trusted markup | Found | Found | Found |
| Preprocess hook → trusted markup | Found | Found | Found |
| Request helper → inline template | Missed | Found | Found |
| Required route argument → direct HTML output | Missed | Missed | Found |
| Queue payload → concatenated SQL | Missed | Missed | Missed |
| `Html::escape`, `#plain_text`, filtered plain-string `#markup`, SQL placeholders | No findings | No findings | No findings |

The final column uses explicit synthetic call sites for the fixture's known route and queue inputs. The queue control remains a coverage gap even with an explicit producer. It is not evidence that arbitrary queue data is safe, nor a determination of the root cause inside Psalm/plugin models. Investigate it before offering queue coverage.

The renderer metadata is a deliberately small XML service map for this fixture, not a complete application container dump. Without the renderer's type, an unsafe inline-template flow disappears. Production integration must track unresolved services and model coverage rather than claim a clean scan from an empty report.

## Setup and completeness failures

The upstream kernel autoloader is unsuitable unchanged for this standalone bundle:

- On Drupal 11 it tries to obtain a database connection during container compilation and fails without one.
- On Drupal 10, the separately installed Symfony 7 runtime conflicts with Drupal 10's kernel method signatures before compilation completes. This is the tested standalone-bundle combination; it does not establish that a project-native installation with compatible Symfony dependencies fails the same way.

The experiment's alternative bootstrap only loads the real project autoloader and registers the fixture module's namespace. It does not compile a Drupal container or instantiate controllers. This allows the unmodified upstream taint rules to run without a site or database.

The upstream route generator invokes callbacks with no arguments. The fixture's required route argument yields `TooFewArguments` during ordinary Psalm analysis. The explicit-call experiment supplies that argument from a known request source; this is not a general implementation of Drupal's argument resolver.

Ordinary Psalm also reports that the generated `FormState` object does not implement `FormStateInterface`. The actual PHP runtime confirms that it does. The upstream source stub redeclares `FormState` without its interface; this points to a structural-model defect requiring correction or a verified overlay, not a reason to suppress the diagnostic.

A malformed helper provides a failure control: PHP lint exits 255 and ordinary Psalm reports `ParseError`, while taint-only mode still returns JSON with an unrelated remaining finding and no parser diagnostic. Missing Composer autoloaders fail before producing taint JSON. An adapter must independently check input readiness and parsing, preserve diagnostics, and mark partial results incomplete.

Original Composer/core fingerprints and copied probe source hashes remain unchanged after the experiments, including restoration of the intentionally broken helper and removed private autoloader. Site settings remain absent. Evidence for the recorded runs lives in `/tmp/phpwarden-psalm-drupal10-verified` and `/tmp/phpwarden-psalm-drupal11-verified` on the validation machine.

## Decision

Reuse of the upstream rules is promising: real Drupal APIs work for several important positive and safe controls. The pinned package is not ready to be bundled unchanged into the normal check pipeline. The remaining work is a database-free profile bootstrap, dependable service/entrypoint models, structural stub correction, queue-flow investigation, and a readiness/completeness contract with failure fixtures. Verify further PHP/runtime combinations and dependency/license distribution requirements separately.

No production checker, defaults, toolset revision, worker image, project hook or agent installation was changed by this experiment.
