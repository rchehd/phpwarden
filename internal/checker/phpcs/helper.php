<?php

declare(strict_types=1);

use Composer\InstalledVersions;
use PHP_CodeSniffer\Config;
use PHP_CodeSniffer\Runner;

try {
    $bundle = $argv[1];
    require $bundle . '/vendor/autoload.php';
    require $bundle . '/vendor/squizlabs/php_codesniffer/autoload.php';
    class PHPWardenJSON extends \PHP_CodeSniffer\Reports\Json
    {
        public function generateFileReport(array $report, \PHP_CodeSniffer\Files\File $phpcsFile, bool $showSources = false, int $width = 80)
        {
            $messages = [];
            foreach ($report['messages'] as $line => $columns) {
                foreach ($columns as $column => $errors) {
                    foreach ($errors as $error) {
                        $messages[] = array_merge($error, ['line' => (int) $line, 'column' => (int) $column]);
                    }
                }
            }
            echo json_encode($report['filename'], JSON_THROW_ON_ERROR), ':', json_encode([
                'errors' => $report['errors'], 'warnings' => $report['warnings'], 'messages' => $messages,
            ], JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE), ',';
            return true;
        }
    }
    foreach (['tokenizer', 'mbstring', 'simplexml', 'xmlwriter'] as $extension) {
        if (!extension_loaded($extension)) {
            throw new RuntimeException('PHPCS requires extension ' . $extension);
        }
    }
    $request = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    $xml = new XMLWriter();
    $xml->openMemory();
    $xml->startDocument('1.0', 'UTF-8');
    $xml->startElement('ruleset');
    $xml->writeAttribute('name', 'PHPWarden');
    foreach ($request['standards'] as $standard) {
        $xml->startElement('rule');
        $xml->writeAttribute('ref', $standard);
        $xml->endElement();
    }
    $xml->endElement();
    $xml->endDocument();
    $ruleset = __DIR__ . '/ruleset.xml';
    file_put_contents($ruleset, $xml->outputMemory());
    $_SERVER['argv'] = array_merge(['phpcs', '--standard=' . $ruleset], $request['args'], $request['files']);
    ob_start();
    $runner = new Runner();
    $exit = $runner->runPHPCS();
    $output = ob_get_clean();
    if ($exit < 0 || $exit > 3) {
        throw new RuntimeException('PHPCS exited ' . $exit . ': ' . trim($output));
    }
    $report = json_decode($output, true, 512, JSON_THROW_ON_ERROR);
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'phpcs_version' => Config::VERSION,
        'coder_version' => ltrim(InstalledVersions::getPrettyVersion('drupal/coder'), 'v'),
        'compatibility_version' => ltrim(InstalledVersions::getPrettyVersion('phpcompatibility/php-compatibility'), 'v'),
        'exit_code' => $exit,
        'report' => $report,
    ], JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE), "\n";
} catch (Throwable $error) {
    while (ob_get_level() > 0) {
        ob_end_clean();
    }
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(70);
}
