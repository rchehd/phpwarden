<?php

declare(strict_types=1);

use Composer\InstalledVersions;
use Symfony\Component\Yaml\Exception\ParseException;
use Symfony\Component\Yaml\Yaml;

try {
    if (!isset($argv[1]) || !is_file($argv[1])) {
        throw new RuntimeException('Bundled YAML dependencies are unavailable.');
    }
    require $argv[1];
    $version = ltrim(InstalledVersions::getPrettyVersion('symfony/yaml'), 'v');
    $request = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    $findings = [];
    foreach ($request['files'] as $path) {
        if (!is_file($path) || !is_readable($path)) {
            throw new RuntimeException('Cannot read YAML file ' . $path);
        }
        try {
            Yaml::parseFile($path, Yaml::PARSE_CUSTOM_TAGS, 128, 128);
        } catch (ParseException $error) {
            $error->setSnippet('');
            $findings[] = [
                'path' => $path,
                'line' => $error->getParsedLine() > 0 ? $error->getParsedLine() : null,
                'message' => $error->getMessage(),
            ];
        }
    }
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'yaml_version' => $version,
        'findings' => $findings,
    ], JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE), "\n";
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(70);
}
