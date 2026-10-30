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
is changed by this bundle candidate. Drupal preparation and the guarded analysis
pipeline are documented in [drupal/README.md](drupal/README.md).
