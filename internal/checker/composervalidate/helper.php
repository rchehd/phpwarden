<?php

declare(strict_types=1);

use Composer\Composer;
use Composer\Factory;
use Composer\IO\NullIO;
use Composer\Package\Loader\ValidatingArrayLoader;
use Composer\Util\ConfigValidator;

try {
    $bundle = $argv[1];
    require $bundle . '/vendor/autoload.php';
    $request = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    $io = new NullIO();
    $clean = static fn (array $messages): array => array_values(array_filter(array_map(
        static fn (string $message): string => trim(preg_replace('/^- /', '', strip_tags($message))),
        $messages
    ), static fn (string $message): bool => '' !== $message));
    $files = [];
    ob_start();
    foreach ($request['files'] as $item) {
        [$errors, , $warnings] = (new ConfigValidator($io))->validate($item['path'], ValidatingArrayLoader::CHECK_ALL, ConfigValidator::CHECK_VERSION);
        $lock = [];
        if ($item['lock'] && [] === $errors) {
            chdir(dirname($item['path']));
            $composer = Factory::create($io, $item['path'], true, true);
            $locker = $composer->getLocker();
            if ($locker->isLocked() && !$locker->isFresh()) {
                $lock[] = 'The lock file is not up to date with the latest changes in composer.json; run composer update.';
            }
            if ($locker->isLocked()) {
                $lock = array_merge($lock, $locker->getMissingRequirementInfo($composer->getPackage(), true));
            }
        }
        $files[$item['path']] = ['errors' => $clean($errors), 'warnings' => $clean($warnings), 'lock' => $clean($lock)];
    }
    if ('' !== ob_get_clean()) {
        throw new RuntimeException('Composer produced unexpected output');
    }
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'composer_version' => Composer::getVersion(),
        'files' => (object) $files,
    ], JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE), "\n";
} catch (Throwable $error) {
    while (ob_get_level() > 0) {
        ob_end_clean();
    }
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(70);
}
