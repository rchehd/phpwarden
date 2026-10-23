<?php

declare(strict_types=1);

use Composer\InstalledVersions;
use TwigCsFixer\Config\Config;
use TwigCsFixer\Environment\StubbedEnvironment;
use TwigCsFixer\Ruleset\Ruleset;
use TwigCsFixer\Runner\Linter;
use TwigCsFixer\Runner\Fixer;
use TwigCsFixer\Standard\TwigCsFixer;
use TwigCsFixer\Token\Tokenizer;

try {
    $bundle = $argv[1];
    require $bundle . '/vendor/autoload.php';
    $request = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    ob_start();
    if (null !== $request['config']) {
        $config = (static fn (string $path): mixed => require $path)($request['config']);
        if (!$config instanceof Config) {
            throw new RuntimeException('Twig CS Fixer config must return ' . Config::class);
        }
    } else {
        $ruleset = new Ruleset();
        $ruleset->addStandard(new TwigCsFixer());
        foreach ($request['removed_rules'] as $rule) {
            $ruleset->removeRule($rule);
        }
        $config = (new Config())->setRuleset($ruleset);
    }
    $ruleset = $config->getRuleset()->allowNonFixableRules($config->areNonFixableRulesAllowed());
    $twig = new StubbedEnvironment($config->getTwigExtensions(), $config->getTokenParsers(), $config->getNodeVisitors());
    $report = (new Linter($twig, new Tokenizer($twig)))->run(array_map(static fn (string $path): SplFileInfo => new SplFileInfo($path), $request['files']), $ruleset);
    $edits = [];
    if ($request['fix'] ?? false) {
        foreach ($request['files'] as $path) {
            $before = file_get_contents($path);
            $after = (new Fixer(new Tokenizer($twig)))->fixFile($before, $ruleset);
            if ($before !== $after) {
                $edits[] = ['path' => $path, 'before' => hash('sha256', $before), 'after' => base64_encode($after)];
            }
        }
    }
    if ('' !== ob_get_clean()) {
        throw new RuntimeException('Twig CS Fixer produced unexpected output');
    }
    $files = [];
    foreach ($request['files'] as $path) {
        $files[$path] = [];
        foreach ($report->getFileViolations($path) as $violation) {
            $id = $violation->getIdentifier();
            $files[$path][] = [
                'level' => $violation->getLevel(),
                'rule' => rtrim(($id?->getRuleIdentifier() ?? '') . '.' . ($id?->getMessageIdentifier() ?? ''), '.'),
                'message' => $violation->getMessage(),
                'line' => $violation->getLine(),
                'column' => $violation->getLinePosition(),
            ];
        }
    }
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'tools' => [
            'vincentlanglet/twig-cs-fixer' => ltrim(InstalledVersions::getPrettyVersion('vincentlanglet/twig-cs-fixer'), 'v'),
            'twig/twig' => ltrim(InstalledVersions::getPrettyVersion('twig/twig'), 'v'),
        ],
        'files' => (object) $files,
        'edits' => $edits,
    ], JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE), "\n";
} catch (Throwable $error) {
    while (ob_get_level() > 0) {
        ob_end_clean();
    }
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(70);
}
