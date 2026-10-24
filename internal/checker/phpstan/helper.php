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
    if ($request['autoload'] !== '') {
        require $request['autoload'];
        $preload = "<?php\n";
        if (class_exists('PHPStan\\ExtensionInstaller\\GeneratedConfig')) {
            // Bundled extensions follow PHPWarden settings; preserve other project extensions.
            $reflection = new ReflectionClass('PHPStan\\ExtensionInstaller\\GeneratedConfig');
            $constants = $reflection->getConstants();
            foreach ($constants['EXTENSIONS'] as $name => $extension) {
                if (isset($versions[$name])) {
                    unset($constants['EXTENSIONS'][$name]);
                    continue;
                }
                $directory = isset($extension['relative_install_path'])
                    ? dirname($reflection->getFileName()) . '/' . $extension['relative_install_path']
                    : $extension['install_path'];
                $constants['EXTENSIONS'][$name]['install_path'] = realpath($directory) ?: $extension['install_path'];
                unset($constants['EXTENSIONS'][$name]['relative_install_path']);
            }
            $code = "<?php\nnamespace PHPStan\\ExtensionInstaller;\nfinal class GeneratedConfig {\n";
            foreach ($constants as $name => $value) {
                $code .= 'public const ' . $name . ' = ' . var_export($value, true) . ";\n";
            }
            $generated = $temporary . '/extensions.php';
            file_put_contents($generated, $code . "}\n");
            $preload .= 'require ' . var_export($generated, true) . ";\n";
        }
        foreach ($request['preload'] as $filename) {
            $preload .= 'require_once ' . var_export($filename, true) . ";\n";
        }
        file_put_contents($temporary . '/project-autoload.php', $preload);
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
