<?php
declare(strict_types=1);

use Composer\InstalledVersions;
use Nette\Neon\Neon;

try {
    $request = json_decode(stream_get_contents(STDIN), true, flags: JSON_THROW_ON_ERROR);
    $bundle = $argv[1];
    $temporary = $argv[2];
    require $bundle . '/vendor/autoload.php';
    $versions = [];
    foreach (['phpstan/phpstan', 'mglaman/phpstan-drupal', 'phpstan/phpstan-deprecation-rules', 'shipmonk/dead-code-detector', 'phpstan/phpstan-strict-rules'] as $name) {
        $versions[$name] = ltrim(InstalledVersions::getPrettyVersion($name), 'v');
    }
    $literal = static fn(string $value): string => str_replace('%', '%%', $value);
    $includes = array_map($literal, $request['includes']);
    $parameters = [
        'tmpDir' => $literal($temporary . '/cache'),
        'resultCachePath' => $literal($temporary . '/resultCache.php'),
        'parallel' => ['maximumNumberOfProcesses' => 1],
        'phpVersion' => $request['php_version'],
        'fileExtensions' => $request['extensions'],
    ];
    if ($request['diff']) {
        $parameters['reportUnmatchedIgnoredErrors'] = false;
    }
    if ($request['exclude'] !== []) {
        $parameters['excludePaths'] = ['analyse' => array_map($literal, $request['exclude'])];
    }
    if ($request['strict_enabled']) {
        $parameters['strictRules'] = ['allRules' => $request['strict_rules']['all']] + $request['strict_rules']['rules'];
    }
    foreach (['bootstrapFiles', 'scanDirectories', 'stubFiles'] as $key) {
        if ($request[$key] !== []) {
            $parameters[$key] = array_map($literal, $request[$key]);
        }
    }
    file_put_contents($temporary . '/phpstan.neon', Neon::encode(['includes' => $includes, 'parameters' => $parameters], Neon::BLOCK));
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'tools' => $versions,
    ], JSON_THROW_ON_ERROR);
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage());
    exit(70);
}
