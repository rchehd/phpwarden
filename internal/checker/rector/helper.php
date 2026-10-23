<?php
declare(strict_types=1);

use Composer\InstalledVersions;

try {
    require $argv[1] . '/vendor/autoload.php';
    $tools = [];
    foreach (['rector/rector', 'palantirnet/drupal-rector', 'phpstan/phpstan', 'webflo/drupal-finder'] as $name) {
        $tools[$name] = ltrim(InstalledVersions::getPrettyVersion($name), 'v');
    }
    echo json_encode(['version' => 1, 'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION, 'php_version' => PHP_VERSION, 'tools' => $tools], JSON_THROW_ON_ERROR);
} catch (Throwable $error) {
    fwrite(STDERR, 'Cannot load the isolated Rector bundle');
    exit(70);
}
