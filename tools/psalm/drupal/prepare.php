<?php

require '/tool-vendor/autoload.php';
require __DIR__ . '/Input.php';
require __DIR__ . '/Services.php';
require __DIR__ . '/Entrypoints.php';

use PHPWarden\Psalm\Drupal\Entrypoints;
use PHPWarden\Psalm\Drupal\Input;
use PHPWarden\Psalm\Drupal\Services;

$output = $argv[2] ?? '';
try {
    if (count($argv) !== 3 || !is_dir($output) || iterator_count(new FilesystemIterator($output)) !== 0) {
        throw new RuntimeException('Usage: prepare.php REQUEST.json EMPTY_OUTPUT_DIRECTORY');
    }
    $request = json_decode(file_get_contents($argv[1]), true, 512, JSON_THROW_ON_ERROR);
    $input = new Input($request);
    $input->read($request['composer_root'] . '/composer.json');
    $lock = json_decode($input->read($request['composer_root'] . '/composer.lock'), true, 512, JSON_THROW_ON_ERROR);
    $versions = array_column($lock['packages'], 'version', 'name');
    $symfony = explode('.', ltrim($versions['symfony/http-kernel'], 'v'))[0];
    $extensions = $input->extensions();
    $files = [];
    $services = [$request['web_root'] . '/core/core.services.yml'];
    $routes = [];
    foreach ($request['source_roots'] as $directory) {
        $files = array_merge($files, $input->files($directory));
    }
    $files = array_values(array_unique($files));
    sort($files);
    foreach ($extensions as $extension) {
        foreach (['services' => &$services, 'routing' => &$routes] as $kind => &$list) {
            $file = $extension['path'] . '/' . $extension['name'] . '.' . $kind . '.yml';
            if (in_array($file, $files, true)) {
                $list[] = $file;
            }
        }
        unset($list);
    }
    $metadata = new Services($input, $services);
    $entries = new Entrypoints($input, $metadata, $files);
    foreach ($routes as $file) {
        $entries->routing($file);
    }
    if (!$entries->sources) {
        $input->diagnostic('no-source', '', 'No selected PHP source files');
    }
    $autoload = "<?php\n\$loader = require " . var_export($input->path($request['composer_root'] . '/vendor/autoload.php'), true) . ";\n";
    foreach ($extensions as $extension) {
        $directory = $extension['path'] . '/src';
        if (is_dir($input->root . '/' . $directory)) {
            $autoload .= '$loader->addPsr4(' . var_export('Drupal\\' . $extension['name'] . '\\', true) . ', ' . var_export($input->path($directory), true) . ");\n";
        }
    }
    $autoload .= "return \$loader;\n";
    $document = new DOMDocument('1.0', 'UTF-8');
    $psalm = $document->appendChild($document->createElement('psalm'));
    foreach (['errorLevel' => '6', 'resolveFromConfigFile' => 'true', 'cacheDirectory' => '/tmp/psalm-cache', 'autoloader' => $output . '/bootstrap.php'] as $name => $value) {
        $psalm->setAttribute($name, $value);
    }
    $extensionsNode = $psalm->appendChild($document->createElement('fileExtensions'));
    foreach (['php', 'module', 'theme', 'inc', 'install', 'profile'] as $extension) {
        $extensionsNode->appendChild($document->createElement('extension'))->setAttribute('name', $extension);
    }
    $project = $psalm->appendChild($document->createElement('projectFiles'));
    foreach ($entries->sources as $file) {
        $project->appendChild($document->createElement('file'))->setAttribute('name', $input->path($file));
    }
    $project->appendChild($document->createElement('file'))->setAttribute('name', $output . '/entrypoints.php');
    $psalm->appendChild($document->createElement('stubs'))->appendChild($document->createElement('file'))->setAttribute('name', $output . '/instances.php');
    $plugins = $psalm->appendChild($document->createElement('plugins'));
    foreach (['Psalm\\SymfonyPsalmPlugin\\Plugin', 'mortenson\\PsalmPluginDrupal\\Plugin', 'PHPWarden\\PsalmSpike\\Plugin'] as $class) {
        $plugin = $plugins->appendChild($document->createElement('pluginClass'));
        $plugin->setAttribute('class', $class);
        if ($class === 'Psalm\\SymfonyPsalmPlugin\\Plugin') {
            $plugin->appendChild($document->createElement('symfonyMajorVersion', $symfony));
        }
        if ($class !== 'PHPWarden\\PsalmSpike\\Plugin') {
            $plugin->appendChild($document->createElement('containerXml', $output . '/services.xml'));
        }
    }
    foreach (['psalm.xml' => $document->saveXML(), 'bootstrap.php' => $autoload, 'services.xml' => $metadata->xml(), 'entrypoints.php' => $entries->calls, 'instances.php' => $entries->stubs] as $name => $text) {
        if (file_put_contents($output . '/' . $name, $text) === false) {
            throw new RuntimeException('Cannot write prepared file: ' . $name);
        }
    }
    ksort($input->hashes);
    $manifest = ['version' => 1, 'preparation_complete' => !$input->diagnostics, 'analysis_complete' => false,
        'selection' => 'installed custom sources; enabled-site state is not inferred', 'extensions' => $extensions,
        'sources' => $entries->sources, 'routes' => $entries->routes, 'services' => $metadata->types,
        'unresolved_services' => $metadata->unresolved, 'diagnostics' => $input->diagnostics, 'inputs' => $input->hashes];
    file_put_contents($output . '/manifest.json', json_encode($manifest, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR) . "\n");
    echo json_encode($manifest, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR) . "\n";
    exit($input->diagnostics ? 3 : 0);
} catch (Throwable $error) {
    fwrite(STDERR, get_class($error) . ': ' . $error->getMessage() . "\n");
    exit(3);
}
