<?php
declare(strict_types=1);

require 'phar:///usr/local/lib/phpwarden/mutation/infection.phar/vendor/autoload.php';
$plan = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
$phpunitConfig = $argv[2];
$mutation = $plan['mutation'];
$settings = $mutation['settings'];
$config = [];
$directory = '/workspace/' . $plan['composer_root'];
if ($settings['config'] !== '') {
    $filename = '/workspace/' . $settings['config'];
    $directory = dirname($filename);
    $config = Infected\ColinODell\Json5\Json5Decoder::decode(file_get_contents($filename), true);
    if (!is_array($config)) {
        throw new RuntimeException('Infection configuration must be an object.');
    }
}
if (!isset($config['source'])) {
    $config['source'] = ['directories' => array_map(static fn ($path) => '/workspace/' . $path, $mutation['source_roots'])];
}
foreach ($config['source']['directories'] ?? [] as $path) {
    $resolved = realpath(str_starts_with($path, '/') ? $path : $directory . '/' . $path);
    if ($resolved === false || !str_starts_with($resolved . '/', '/workspace/')) {
        throw new RuntimeException('Infection source directories must stay inside the project.');
    }
}
$config['source']['excludes'] ??= [];
foreach ($plan['coverage_exclude_directories'] ?? [] as $name) {
    $config['source']['excludes'][] = '{(^|/)' . preg_quote($name, '{') . '(/|$)}';
}
$config['source']['excludes'] = array_values(array_unique($config['source']['excludes']));
$config['logs'] = ['json' => '/artifacts/infection.json', 'html' => '/artifacts/infection.html'];
$config['tmpDir'] = '/tmp/phpwarden-infection';
$config['threads'] = $settings['threads'] ?? $config['threads'] ?? 1;
$config['timeout'] = $settings['mutant_timeout_seconds'] ?? $config['timeout'] ?? 10;
foreach (['initialTestsPhpOptions', 'testFrameworkOptions', 'testFrameworkExtraArgs', 'staticAnalysisTool', 'staticAnalysisToolOptions'] as $key) {
    if (isset($config[$key])) {
        throw new RuntimeException('Use tests.* for test selection; raw test-framework/static-analysis options are not supported: ' . $key);
    }
}
if (!empty($settings['mutators'])) {
    $config['mutators'] = array_merge(['@default' => false], array_fill_keys($settings['mutators'], true));
}
$config['mutators'] ??= ['@default' => true];
$minimum = $settings['min_msi'] ?? $config['minMsi'] ?? null;
$coveredMinimum = $settings['min_covered_msi'] ?? $config['minCoveredMsi'] ?? null;
unset($config['minMsi'], $config['minCoveredMsi']);
if (isset($config['testFramework']) && $config['testFramework'] !== 'phpunit') {
    throw new RuntimeException('PHPWarden mutation currently supports PHPUnit only.');
}
$config['testFramework'] = 'phpunit';
$config['phpUnit'] = ['configDir' => dirname($phpunitConfig), 'customPath' => '/workspace/' . $plan['phpunit']];
if (!copy($phpunitConfig, dirname($phpunitConfig) . '/phpunit.xml')) {
    throw new RuntimeException('Could not prepare private PHPUnit config for Infection.');
}
$coverage = '/artifacts/infection-coverage';
mkdir($coverage);
symlink('../coverage/xml', $coverage . '/coverage-xml');
copy('/artifacts/phpunit-001.xml', $coverage . '/junit.xml');
$output = tempnam($directory, '.phpwarden-mutation-');
file_put_contents($output, json_encode($config, JSON_THROW_ON_ERROR | JSON_PRETTY_PRINT));
echo json_encode(['config' => $output, 'min_msi' => $minimum, 'min_covered_msi' => $coveredMinimum], JSON_THROW_ON_ERROR);
