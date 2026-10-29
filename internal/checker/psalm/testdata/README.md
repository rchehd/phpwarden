# Native Drupal taint reports

These four captures come from the verified 2026-10-29 worker acceptance runs:
Drupal 10.6.0 and 11.4.1, each with a normal layout and a monorepo Composer path
package outside the web root. Psalm 6.20.3 and the pinned upstream plugins ran on
PHP 8.3.36. Each report contains seven findings across six unsafe behaviors;
four safe controls produced none. These files test normalization, not a new
native analyzer run.

Findings retain their native fields and ordered traces. Only the random generated
workspace root was replaced with `/generated`. The source and context inventories
come from each preparation manifest; `./` prefixes were canonicalized. Tool paths
remain under `/tool-vendor`, project paths under `/project`.

Original phase output and manifests are preserved in the local evidence archive
`docs/artifacts/psalm-worker-2026-10-29/native-acceptance.tar.gz`. The runtime
pipeline supplies these roots and inventories directly in memory, without adding
them to the native evidence JSON format.
