<?php
declare(strict_types=1);

use Composer\InstalledVersions;
use Symfony\Component\Console\Application;
use Symfony\Component\Console\Input\ArrayInput;
use Symfony\Component\Console\Output\BufferedOutput;

try {
    $request = json_decode(stream_get_contents(STDIN), true, flags: JSON_THROW_ON_ERROR);
    $bundle = $argv[1];
    require $bundle . '/vendor/autoload.php';
    $unused = $request['id'] === 'composer-unused';
    $package = $unused ? 'icanhazstring/composer-unused' : 'maglnet/composer-require-checker';
    $version = ltrim(InstalledVersions::getPrettyVersion($package), 'v');
    if ($version !== $request['tool_version']) {
        throw new RuntimeException('Unexpected dependency checker version');
    }
    foreach ($request['requires'] as $name) {
        if (str_starts_with($name, 'ext-') && !extension_loaded(substr($name, 4))) {
            throw new RuntimeException('Required PHP extension is missing');
        }
    }
    if ($unused) {
        $installed = require $request['vendor'] . '/composer/installed.php';
        foreach ($request['requires'] as $name) {
            if (!str_contains($name, '/')) {
                continue;
            }
            $actual = $installed['versions'][$name]['install_path'] ?? null;
            if (!is_string($actual) || realpath($actual) !== realpath($request['vendor'] . '/' . $name)) {
                throw new RuntimeException('Installed package paths do not match the supported layout');
            }
        }
    }
    $application = new Application($request['id'], $version);
    $application->setAutoExit(false);
    $application->setCatchExceptions(false);
    $arguments = ['command' => $unused ? 'unused' : 'check', 'composer-json' => $request['composer'], '--no-interaction' => true, '--no-ansi' => true];
    if ($unused) {
        define('UNUSED_COMPOSER_INSTALL', $bundle . '/vendor/autoload.php');
        $container = require $bundle . '/vendor/icanhazstring/composer-unused/config/container.php';
        $application->add($container->get(ComposerUnused\ComposerUnused\Console\Command\UnusedCommand::class));
        $configuration = $request['config'] === '' ? 'new \\ComposerUnused\\ComposerUnused\\Configuration\\Configuration()' : '(require ' . var_export($request['config'], true) . ')($config)';
        $wrapper = '<?php return static function ($config) { $config = ' . $configuration . '; return $config; };';
        file_put_contents(__DIR__ . '/composer-unused.php', $wrapper);
        $arguments += ['command' => 'unused', '--output-format' => 'json', '--no-progress' => true, '--configuration' => __DIR__ . '/composer-unused.php', '--excludePackage' => $request['exclude_packages'], '--excludeDir' => $request['exclude_directories']];
    } else {
        $application->add(new ComposerRequireChecker\Cli\CheckCommand());
        $options = $request['config'] === '' ? [] : json_decode(file_get_contents($request['config']), true, flags: JSON_THROW_ON_ERROR);
        $options['scan-files'] = array_merge($options['scan-files'] ?? [], $request['scan_files']);
        $options['symbol-whitelist'] = array_merge($options['symbol-whitelist'] ?? [], $request['allowed_symbols']);
        file_put_contents(__DIR__ . '/require-checker.json', json_encode($options, JSON_THROW_ON_ERROR));
        $arguments += ['command' => 'check', '--output' => 'json', '--config-file' => __DIR__ . '/require-checker.json'];
    }
    $output = new BufferedOutput();
    $status = $application->run(new ArrayInput($arguments), $output);
    $data = json_decode($output->fetch(), true, flags: JSON_THROW_ON_ERROR);
    echo json_encode(['version' => 1, 'tool_version' => $version, 'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION, 'php_version' => PHP_VERSION, 'status' => $status, 'data' => $data], JSON_THROW_ON_ERROR);
} catch (Throwable $error) {
    fwrite(STDERR, 'Dependency analysis could not complete; no partial results accepted');
    exit(70);
}
