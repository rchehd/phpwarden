# Psalm taint feasibility spike

This development harness evaluates a possible opt-in security integration. It does not register a PHPWarden checker, change project configuration, or add Psalm to worker images. Run it on disposable fixtures; it never mounts a working application.

## Reproduce

Download the [official Psalm 6.20.3 PHAR](https://github.com/vimeo/psalm/releases/tag/6.20.3). The harness verifies SHA-256 `bda19a8cbe8914a7eab079c4a33d25e69ca20856e5e0c4ae5c9dcab1d8ecc828` before executing it. Supply an existing local image containing compatible PHP and Composer; images are never pulled or built.

```sh
python3 tools/psalm-spike/run.py \
  --phar /tmp/phpwarden-psalm-6.20.3.phar \
  --image ddev/ddev-webserver:v1.25.5 \
  --php /usr/bin/php8.3 \
  --output /tmp/phpwarden-psalm-new-run
```

Each fixture gets its own dependency-free Composer manifest and generated autoloader. Composer only dumps autoload metadata, with plugins/scripts and network disabled; no packages are installed. Psalm runs with read-only fixtures, a temporary cache, one thread, no network and no capabilities. Output includes the original JSON, stderr, source preservation checks and `summary.json`. The output directory must be new. Python and Docker are development dependencies for this harness.

## Observed results

Validated on PHP 8.3 with Psalm 6.20.3, 2026-10-29:

| Fixture | Observed native result |
| --- | --- |
| Direct request value → echo | `TaintedHtml` and `TaintedTextWithQuotes` |
| Request source inside a helper file → returned value → echo in another file | Same findings; JSON retains the flow trace |
| Helper result → `htmlspecialchars(..., ENT_QUOTES, 'UTF-8')` → echo | No findings |
| Framework-shaped request → helper → trusted markup, without models | No findings: intentional false-negative control |
| Same flow with explicit source/sink stubs | `TaintedHtml` |
| Modeled flow escaped with the modeled HTML sanitizer | No findings |

These are small API-shaped doubles, not a Drupal compatibility test. Their bodies are not Drupal implementations. Moving framework declarations outside the custom source scope and providing real Composer mappings matters: missing definitions or analyzed placeholder bodies can hide a flow. An empty taint report alone cannot establish coverage. No successful Composer/platform/install assumptions should be synthesized for a real project.

The bundled [Semgrep rules](../../internal/checker/semgrep/rules.yaml) already cover direct request-to-sink patterns. A direct comparison used the same pinned CE engine 1.182.0 and rules 1.0.0, scanned per file as the adapter does, with YAML aliases resolved before native execution. Both engines found the direct case and accepted the escaped control. Only Psalm found the flow whose request source is inside another file. This is additional coverage for that fixture, not a performance comparison or application security benchmark. PHPStan remains the existing type/correctness analyzer; this experiment uses Psalm only for taint analysis.

To repeat the comparison, use a prepared native engine from the [Semgrep bundle](../semgrep/README.md). The optional comparison script needs PyYAML in its development Python environment. Supply the bundle library directory when the engine requires it; the script sets its loader path only for native subprocesses.

```sh
python3 tools/psalm-spike/compare.py \
  --reports /tmp/phpwarden-psalm-new-run \
  --semgrep-core /path/to/semgrep/vendor/bin/semgrep-core \
  --library-dir /path/to/semgrep/vendor/bin/libs
```

`comparison.json` retains issue IDs from both tools. Native errors/skipped rules/targets fail the comparison; a broken scan is not counted as an absence of findings.

## Drupal integration decision

The follow-up [real Drupal compatibility experiment](DRUPAL.md) tests the pinned upstream plugin on Drupal 10/11 without an installed site or database. It identifies additional bootstrap, service typing, entrypoint and completeness gaps. The small doubles above do not establish production readiness.

Psalm is worth a separate opt-in security integration. Keep it advisory initially, with full-graph analysis and its own native configuration/baseline. Do not enable it by default on Drupal on the basis of these fixtures.

Reuse and evaluate [mortenson/psalm-plugin-drupal](https://github.com/mortenson/psalm-plugin-drupal/tree/073e7a4d5fc159fbf47decf16296edd7f8a545d1), which already supplies Drupal source/sink/sanitizer stubs, render-array handling, module/theme discovery, service handling and an entrypoint generator. Its pinned revision requires Psalm `^6` and `psalm/plugin-symfony ^5.0`; its latest change mentions Drupal 11.1.6 tests. This does not establish compatibility with every supported Drupal/PHP combination. Check its broad file discovery, optional container XML handling and generated entrypoint coverage before bundling it. Route-derived entrypoints alone do not prove coverage of hooks, plugins, forms or queue workers.

Next implementation gate:

1. Pin and test the real plugin with installed, locked Drupal 10/11 dependencies in private copies. Verify request sources, dynamic entrypoints, render arrays, parameterized SQL and context-specific escaping using positive and negative fixtures. No site/container dump should become an implicit prerequisite.
2. Check class/function resolution and scan diagnostics separately from taint findings. Missing dependencies, unmodeled entrypoints and parser/runtime/report failures must not produce a clean complete verdict.
3. Preserve full native traces and report context. Define changes-mode reporting explicitly: analyze the whole graph, then retain affected flows, including changes at a source/helper with a sink outside changed lines. Do not run Psalm only on changed files and claim complete security coverage.
4. Package the pinned tool/plugin and required dependency notices; add native configuration overrides, bounded runtime/memory, cancellation and failure fixtures before registering the checker.

The pinned Psalm [manifest](https://github.com/vimeo/psalm/blob/6.20.3/composer.json) allows `~8.1.31 || ~8.2.27 || ~8.3.16 || ~8.4.3 || ~8.5.0` and requires XML/DOM/mbstring/tokenizer extensions. This spike validates PHP 8.3 only; it does not certify other worker runtimes. Psalm itself is MIT; the Drupal plugin is LGPL-2.1-only, and its dependencies require their own license review and notices. No upstream code or PHAR is vendored by this spike.

Native configuration and annotations are documented in [Psalm security analysis](https://psalm.dev/docs/security_analysis/), [custom sources](https://psalm.dev/docs/security_analysis/custom_taint_sources/), [custom sinks](https://psalm.dev/docs/security_analysis/custom_taint_sinks/) and [avoiding false positives](https://psalm.dev/docs/security_analysis/avoiding_false_positives/). Documentation can describe newer versions; verify behavior against the pinned release.
