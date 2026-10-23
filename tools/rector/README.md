# Drupal Rector bundle

This directory locks Drupal Rector 1.1.3 with Rector 2.6.1, PHPStan 2.3.1, and Drupal Finder 1.3.1. It has its own vendor directory and does not load PHPWarden's primary PHPStan bundle. Drupal Rector 1.1.3 conflicts with Rector >=2.6.2; do not update Rector independently.

Dependencies are installed only during an authorized build or test preparation:

```sh
composer install --working-dir tools/rector --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction
```

The lock was resolved without installation. Native integration and the supported PHP runtime matrix still need the combined validation run. PHPWarden invokes the pinned JSON CLI with forced dry-run, scoped inputs, sequential processing, a memory limit and private caches. The packaged worker mounts project files read-only.

Official sources: [Drupal Rector 1.1.3](https://github.com/palantirnet/drupal-rector/tree/1.1.3), [Rector 2.6.1](https://github.com/rectorphp/rector/tree/2.6.1), [Rector configuration](https://getrector.com/documentation/config-configuration). Composer keeps upstream license files in the isolated bundle.
