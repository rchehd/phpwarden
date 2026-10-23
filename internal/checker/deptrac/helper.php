<?php
declare(strict_types=1);

use Composer\InstalledVersions;
use Deptrac\Deptrac\Contract\Ast\AstFileAnalysedEvent;
use Deptrac\Deptrac\Contract\Ast\AstFileSyntaxErrorEvent;
use Deptrac\Deptrac\Contract\Ast\ParserInterface;
use Deptrac\Deptrac\Contract\Result\OutputResult;
use Deptrac\Deptrac\Contract\Result\SkippedViolation;
use Deptrac\Deptrac\Contract\Result\Uncovered;
use Deptrac\Deptrac\Contract\Result\Violation;
use Deptrac\Deptrac\Core\Analyser\DependencyLayersAnalyser;
use Deptrac\Deptrac\Core\InputCollector\FileInputCollector;
use Deptrac\Deptrac\DefaultBehavior\Ast\Parser\NikicPhpParser;
use Deptrac\Deptrac\Supportive\DependencyInjection\ServiceContainerBuilder;
use Symfony\Component\EventDispatcher\EventDispatcher;

try {
    $request = json_decode(stream_get_contents(STDIN), true, flags: JSON_THROW_ON_ERROR);
    require $argv[1] . '/vendor/autoload.php';
    $tools = [];
    foreach (['deptrac/deptrac', 'phpstan/phpstan'] as $name) {
        $tools[$name] = ltrim(InstalledVersions::getPrettyVersion($name), 'v');
    }
    if ($tools !== ['deptrac/deptrac' => '4.4.0', 'phpstan/phpstan' => '2.3.1']) {
        throw new RuntimeException('Unexpected Deptrac bundle version');
    }
    $wrapper = [
        'imports' => [['resource' => $request['config']]],
        'parameters' => ['projectDirectory' => dirname($request['config'])],
        'services' => [
            ParserInterface::class => ['alias' => NikicPhpParser::class],
            'phpwarden.analyser' => ['alias' => DependencyLayersAnalyser::class, 'public' => true],
            'phpwarden.inputs' => ['alias' => FileInputCollector::class, 'public' => true],
            'phpwarden.events' => ['alias' => EventDispatcher::class, 'public' => true],
        ],
    ];
    $filename = __DIR__ . '/deptrac.yaml';
    file_put_contents($filename, json_encode($wrapper, JSON_THROW_ON_ERROR));
    $container = (new ServiceContainerBuilder($request['project_root']))->withConfig($filename)->build(false, false);
    $prefix = rtrim($request['project_root'], '/') . '/';
    $relative = static function (string $filename) use ($prefix): string {
        $resolved = realpath($filename);
        if ($resolved === false || !str_starts_with($resolved, $prefix)) {
            throw new RuntimeException('Deptrac graph escaped the project');
        }
        return substr($resolved, strlen($prefix));
    };
    $files = array_map($relative, $container->get('phpwarden.inputs')->collect());
    sort($files);
    $parsed = [];
    $events = $container->get('phpwarden.events');
    $events->addListener(AstFileSyntaxErrorEvent::class, static function (): void {
        throw new RuntimeException('Deptrac graph contains an unreadable or unparsable file');
    });
    $events->addListener(AstFileAnalysedEvent::class, static function (AstFileAnalysedEvent $event) use (&$parsed, $relative): void {
        $parsed[] = $relative($event->file);
    });
    $result = OutputResult::fromAnalysisResult($container->get('phpwarden.analyser')->analyse());
    sort($parsed);
    if ($parsed !== $files) {
        throw new RuntimeException('Deptrac did not parse the complete configured graph');
    }
    $findings = [];
    foreach ([Violation::class => 'violation', Uncovered::class => 'uncovered', SkippedViolation::class => 'skipped'] as $class => $kind) {
        foreach ($result->allOf($class) as $rule) {
            $dependency = $rule->getDependency();
            $occurrence = $dependency->getContext()->fileOccurrence;
            $findings[] = [
                'file' => $relative($occurrence->filepath),
                'line' => $occurrence->line,
                'kind' => $kind,
                'depender' => $dependency->getDepender()->toString(),
                'dependent' => $dependency->getDependent()->toString(),
                'from_layer' => $rule instanceof Uncovered ? $rule->layer : $rule->getDependerLayer(),
                'to_layer' => $rule instanceof Uncovered ? null : $rule->getDependentLayer(),
            ];
        }
    }
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'tools' => $tools,
        'files' => $files,
        'counts' => ['violation' => count($result->violations()), 'uncovered' => count($result->uncovered()), 'skipped' => count($result->skippedViolations()), 'allowed' => count($result->allowed())],
        'errors' => count($result->errors),
        'warnings' => array_map(static fn($warning): string => (string) $warning, $result->warnings),
        'findings' => $findings,
    ], JSON_THROW_ON_ERROR | JSON_INVALID_UTF8_SUBSTITUTE);
} catch (Throwable $error) {
    fwrite(STDERR, 'Deptrac could not complete the configured architecture analysis');
    exit(70);
}
