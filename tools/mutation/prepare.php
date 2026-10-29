<?php
declare(strict_types=1);

$archive = new Phar('/out/infection.phar');
$count = 0;
foreach (new RecursiveIteratorIterator($archive) as $file) {
    if (!preg_match('/^(LICENSE|COPYING|NOTICE|PATENTS)/i', $file->getFilename())) {
        continue;
    }
    $relative = substr($file->getPathname(), strlen('phar:///out/infection.phar/'));
    $output = '/out/notices/' . $relative;
    if (!is_dir(dirname($output))) {
        mkdir(dirname($output), 0755, true);
    }
    file_put_contents($output, file_get_contents($file->getPathname()));
    $count++;
}
if ($count < 20) {
    throw new RuntimeException('Infection dependency notices are missing.');
}
