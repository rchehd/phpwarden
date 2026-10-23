# Deptrac bundle

This isolated bundle pins Deptrac 4.4.0 and PHPStan 2.3.1, with every transitive dependency in `composer.lock`. Deptrac 4.4.0 supports PHP 8.1 and is the newest release usable across the full worker runtime matrix; later versions require PHP 8.2. Bundle dependency identities use a `deptrac-` manifest prefix so they do not conflict with the other isolated tools. The manifest normalizes the JetBrains `v2024.3` calendar tag to `2024.3.0`; the lock retains the exact upstream tag and revision.

Install only during an authorized build or test preparation:

```sh
composer install --working-dir tools/deptrac --no-dev --no-plugins --no-scripts --prefer-dist --no-interaction
```

The lock was resolved without installation. PHPWarden uses the pinned `ServiceContainerBuilder` and `DependencyLayersAnalyser` APIs to preserve the PHP memory limit and obtain typed violations, uncovered dependencies, accepted baseline entries, and warnings/errors. The standard Nikic parser is selected directly; no persistent cache is loaded; generated wrappers and temporary files stay in a private directory. The project is mounted read-only by the worker. The complete configured `.php` graph is analyzed before filtering findings to the requested scope.

Project configuration supports native `deptrac` settings and local YAML imports. PHP configs, custom services/parameters/tags, aliases/anchors, optional/glob/remote imports and the experimental PHPStan parser flag are unsupported. The upstream YAML settings and custom in-memory wrapper are validated separately. Native runtime behavior remains pending the combined build/test run.

Official sources: [Deptrac 4.4.0](https://github.com/deptrac/deptrac/tree/4.4.0), [configuration](https://deptrac.github.io/deptrac/configuration/), [release requirements](https://github.com/deptrac/deptrac/blob/4.4.0/composer.json). Composer retains upstream license files inside the bundle.
