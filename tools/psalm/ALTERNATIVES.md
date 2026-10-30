# Drupal Psalm distribution options

Checked on 2026-10-30 against upstream source/history, Packagist and the current
PHPWarden integration. The distribution model for PHPWarden remains undecided.

The pinned Drupal plugin's [actual LICENSE](https://github.com/mortenson/psalm-plugin-drupal/blob/073e7a4d5fc159fbf47decf16296edd7f8a545d1/LICENSE)
adds Commons Clause to LGPL 2.1. Its [first revision](https://github.com/mortenson/psalm-plugin-drupal/blob/9c97d0354473c61780aaf970ac48337bd29f17bd/LICENSE)
already contains that condition. The GitHub history query for `path=LICENSE`
returned only that initial commit. A version downgrade therefore provides no
identified alternative license. [Packagist](https://packagist.org/packages/mortenson/psalm-plugin-drupal)
reports the shorter Composer declaration and does not resolve that discrepancy.

The search did not establish a maintained, equivalent Drupal Psalm taint plugin
without that condition. This is a research result, not proof none exists.
Forking the same code or editing its license field does not establish new rights.

| Option | Technical consequence | Decision needed |
| --- | --- | --- |
| Keep the current plugin | Preserves the tested Drupal sources/sinks, service typing, render-array behavior and FormState integration. | Assess the intended distribution against the actual condition, or obtain separate permission from its copyright holder. |
| Build independent Drupal models | Reimplement the required public API behavior and validate every positive/safe fixture against Psalm's plugin API. Existing framework-neutral pipeline, reporting, preparation and native config support can remain. | Approve the additional maintenance scope; do not copy/relabel the restricted plugin or its stubs. |
| Ship Psalm without Drupal models | Generic PHP flows still work, but Drupal render arrays, FormState and dynamic entry points lose coverage. | Define and expose that reduced scope; it cannot replace the current Drupal preset silently. |
| Keep Drupal Psalm experimental | Other shipped checks and agent integrations continue to work. The isolated candidate remains available for technical validation. | Defer public integration until one of the other routes is selected. |

Current choice: keep Drupal Psalm experimental while the user decides the
distribution model. Continue bundle acceptance and keep public image integration
as a separate distribution milestone.

An independent replacement would need its own Drupal source/sink/sanitizer
annotations, `Drupal::service()` return typing and render-array taint handling.
The current profile also relies on upstream FormState annotations; its local
empty interface overlay does not supply those semantics by itself. Removing the
dependency or its plugin registration alone would reduce security coverage.
Cutover would require the existing Drupal 10/11 positive and safe-control matrix,
ordinary-readiness failure cases, monorepo discovery, source preservation and
full report/config regression tests before changing any advertised support.
