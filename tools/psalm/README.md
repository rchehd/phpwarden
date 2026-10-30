# Isolated Psalm bundle candidate

`composer.json` and `composer.lock` promote the already tested development pins
to a dedicated tool bundle: Psalm 6.20.3, Drupal plugin revision `073e7a4`,
Symfony plugin 5.3.0 and Symfony YAML 7.4.21. The other 65 locked packages are
unchanged from the compatibility spike. PHP 8.3.16+ in the 8.3 series is the
currently verified integration runtime. No application dependencies are installed
or changed by preparing this separate tool directory.

Install with PHP 8.3 in an isolated directory, with Composer plugins and scripts
disabled. Use the pinned source URLs/checksums in `sources.json` to supply both
archives, then generate the notice inventory offline:

```sh
composer install --working-dir /scratch/psalm --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction
python3 tools/psalm/prepare.py --bundle /scratch/psalm --sources /scratch/sources --output /scratch/psalm-notices
```

The bundle directory needs copies of this `composer.json` and `composer.lock`.
The preparation command checks every installed package's version/revision against
the lock, requires license declarations and actual notice files, verifies both
source archive checksums and compares their LICENSE files to the installed copies.
An existing nonempty output directory is refused. Original vendor files are not
modified. Generated output contains `NOTICE.md`, `inventory.json`, `notices/`
and `sources/`.

**Public packaging is pending a license decision.** The pinned Drupal plugin's
[actual LICENSE](https://github.com/mortenson/psalm-plugin-drupal/blob/073e7a4d5fc159fbf47decf16296edd7f8a545d1/LICENSE)
adds Commons Clause to LGPL 2.1, despite the shorter Composer declaration.
See [NOTICE.md](NOTICE.md). No image capability is advertised and no worker image
is changed by this bundle candidate. Distribution alternatives and their support
tradeoffs are recorded in [ALTERNATIVES.md](ALTERNATIVES.md). Drupal preparation
and the guarded analysis pipeline are documented in [drupal/README.md](drupal/README.md).

Run the native XML/baseline acceptance against the actual installed bundle:

```sh
go build -o /scratch/native-config ./tools/psalm-spike/nativeconfig
python3 tools/psalm-spike/native_config.py --binary /scratch/native-config --bundle /scratch/psalm --image ddev/ddev-webserver:v1.25.5 --php /usr/bin/php8.3 --output /scratch/bundle-acceptance
```

This mode verifies the committed lock and installed package inventory, uses the
bundle's own Psalm entrypoint/schema and checks that bundle contents are unchanged
after all ten native-policy cases. It runs containers sequentially at 0.2 CPU,
without network or image pulls, and removes fixture workspaces after checking for
active mounts. Reports retain bundle identity and case results. `--phar` with
`--schema` remains available for the original checksum-pinned PHAR comparison.
These focused fixtures do not replace the full Drupal/runtime matrix.

The harness records the actual PHP patch version, numeric version ID and loaded
extensions. `--runtime 8.3` or `--runtime 8.4` asserts the requested minor runtime;
a mismatch aborts the run instead of substituting another PHP. On PHP 8.4 it also
runs unsafe and escaped property-hook controls after the ten common cases.

For a PHP 8.4 execution experiment, use `--php /usr/bin/php8.4 --runtime 8.4` with
the same installed candidate. Install that candidate with PHP 8.3, as required by
its current Composer root constraint. The experiment tests execution separately
from installation compatibility. It does not widen the public worker's PHP 8.3
guard or establish Drupal 10/11 model compatibility on PHP 8.4.

The PHP 8.4 gate currently fails an unsafe property-hook control: Psalm 6.20.3
misses the flow while reporting the equivalent ordinary method. The guard remains
closed; see [PHP84.md](PHP84.md) for the measured results and minimal reproducer.
