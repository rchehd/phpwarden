<?php

require '/helper/Input.php';

use PHPWarden\Psalm\Drupal\Input;

$cases = [
    ['**/vendor/**', 'vendor/a.php', true],
    ['**/vendor/**', 'nested/vendor', true],
    ['**/vendor/**', 'nested/vendorish/a.php', false],
    ['src/*.php', 'src/a.php', true],
    ['src/*.php', 'src/nested/a.php', false],
    ['src/**/a?.php', 'src/a1.php', true],
    ['src/**/a?.php', 'src/nested/deep/a1.php', true],
    ['src/**/a?.php', 'src/a12.php', false],
    ['**/.git/**', '.git/config', true],
    ['**/generated/**', 'src/Generated/file.php', false],
    ['packages/outside', 'packages/outside/a.php', false],
    ['packages/outside/**', 'packages/outside/a.php', true],
];
foreach ($cases as [$pattern, $file, $expected]) {
    $input = new Input([
        'root' => '/tmp', 'composer_root' => '.', 'web_root' => '.',
        'source_roots' => [], 'include' => [], 'exclude' => [$pattern],
    ]);
    if ($input->excluded($file) !== $expected || !$input->matches('src/a.php', ['.'])) {
        throw new RuntimeException('Selection mismatch: ' . $pattern . ':' . $file);
    }
}
echo json_encode(['passed' => true, 'boundaries' => count($cases)]) . "\n";
