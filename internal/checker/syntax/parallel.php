<?php
declare(strict_types=1);

use Composer\InstalledVersions;
use JakubOnderka\PhpParallelLint\ParallelLint;
use JakubOnderka\PhpParallelLint\Process\PhpExecutable;
use JakubOnderka\PhpParallelLint\SyntaxError;

ob_start();
try {
    require $argv[1] . '/vendor/autoload.php';
    if (InstalledVersions::getPrettyVersion('php-parallel-lint/php-parallel-lint') !== 'v1.4.0') {
        throw new RuntimeException('Unexpected lint version');
    }
    $input = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    $executable = new PhpExecutable(escapeshellarg(PHP_BINARY) . ' -n -d memory_limit=128M', PHP_VERSION_ID, null, false);
    $engine = new ParallelLint($executable, $input['jobs']);
    $result = $engine->lint($input['files']);
    $checked = $result->getCheckedFiles();
    $expected = $input['files'];
    sort($checked);
    sort($expected);
    if ($checked !== $expected || $result->getSkippedFiles() !== []) {
        throw new RuntimeException('Incomplete lint coverage, including native skip directives');
    }
    $errors = [];
    foreach ($result->getErrors() as $error) {
        if (!$error instanceof SyntaxError || !$error->getLine()) {
            throw new RuntimeException('Lint subprocess failed');
        }
        $message = $error->getMessage();
        if (preg_match('/Allowed memory size|Maximum execution time|Out of memory/', $message)) {
            throw new RuntimeException('Lint resource failure');
        }
        $errors[] = ['file' => $error->getFilePath(), 'line' => $error->getLine(), 'message' => $message];
    }
    if (ob_get_clean() !== '') {
        throw new RuntimeException('Unexpected lint output');
    }
    echo json_encode(['version' => 1, 'tool_version' => '1.4.0', 'php_version' => PHP_VERSION,
        'checked' => $checked, 'errors' => $errors], JSON_THROW_ON_ERROR);
} catch (Throwable $error) {
    while (ob_get_level()) {
        ob_end_clean();
    }
    fwrite(STDERR, "Parallel lint did not complete\n");
    exit(70);
}
