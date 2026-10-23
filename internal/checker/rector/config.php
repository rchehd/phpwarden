<?php
declare(strict_types=1);

use DrupalRector\Services\DrupalRectorSettings;
use Rector\Config\RectorConfig;
use Rector\Configuration\Option;
use Rector\Configuration\Parameter\SimpleParameterProvider;

return static function (RectorConfig $config): void {
    $request = json_decode(file_get_contents(__DIR__ . '/request.json'), true, flags: JSON_THROW_ON_ERROR);
    if ($request['config'] !== '') {
        $config->import($request['config']);
    }
    $config->sets(array_values(array_unique($request['sets'])));
    $config->rules($request['rules']);
    $config->skip(array_fill_keys($request['skip_rules'], $request['files']));
    if ($request['drupal_version'] !== '') {
        $config->singleton(DrupalRectorSettings::class, static fn() => (new DrupalRectorSettings())
            ->disableBackwardCompatibility()
            ->setMinimumCoreVersionSupported($request['drupal_version'])
            ->setDrupalVersion($request['drupal_version']));
    }
    $config->paths($request['files']);
    $config->fileExtensions($request['extensions']);
    $config->phpVersion($request['php_version']);
    $config->memoryLimit($request['memory_limit_mb'] . 'M');
    $config->disableParallel();
    $config->cacheDirectory(__DIR__ . '/cache');
    $config->containerCacheDirectory(__DIR__);
    $config->autoloadPaths([$request['project']]);
    SimpleParameterProvider::setParameter(Option::NO_DIFFS, false);
    SimpleParameterProvider::setParameter(Option::ABSOLUTE_FILE_PATH, true);
    SimpleParameterProvider::setParameter(Option::IS_RUN_NARROWED, true);
};
