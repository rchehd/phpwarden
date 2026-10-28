<?php
declare(strict_types=1);

if (!extension_loaded('pcov') || !ini_get('pcov.enabled')) {
    throw new RuntimeException('Coverage driver PCOV is unavailable; rebuild the worker together with the CLI.');
}
$plan = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
$major = (int) $argv[2];
if ($major < 9 || $major > 12) {
    throw new RuntimeException('Coverage supports PHPUnit 9–12.');
}
$filename = '/workspace/' . $plan['config'];
$xml = new DOMDocument();
if (!$xml->load($filename, LIBXML_NONET) || $xml->doctype !== null || $xml->documentElement->tagName !== 'phpunit') {
    throw new RuntimeException('Invalid native PHPUnit configuration.');
}
$root = $xml->documentElement;
$child = static function (DOMElement $parent, string $name) use ($xml): DOMElement {
    foreach ($parent->childNodes as $node) {
        if ($node instanceof DOMElement && $node->tagName === $name) {
            return $node;
        }
    }
    return $parent->appendChild($xml->createElement($name));
};
$remove = static function (DOMElement $parent, string $name): void {
    foreach (iterator_to_array($parent->childNodes) as $node) {
        if ($node instanceof DOMElement && $node->tagName === $name) {
            $parent->removeChild($node);
        }
    }
};
if (!empty($plan['paths'])) {
    $remove($root, 'testsuites');
    $remove($root, 'testsuite');
    $suite = $child($child($root, 'testsuites'), 'testsuite');
    $suite->setAttribute('name', 'phpwarden-selected');
    foreach ($plan['paths'] as $path) {
        $path = '/workspace/' . $path;
        $node = $suite->appendChild($xml->createElement(is_dir($path) ? 'directory' : 'file'));
        $node->textContent = $path;
        if (is_dir($path)) {
            $node->setAttribute('suffix', 'Test.php');
        }
    }
}
$coverage = $child($root, 'coverage');
$coverage->setAttribute('includeUncoveredFiles', 'true');
$source = $major === 9 ? $coverage : $child($root, 'source');
$options = $plan['coverage'];
if (!empty($options['include'])) {
    $remove($source, 'include');
    $remove($source, 'exclude');
    $include = $child($source, 'include');
    $exclude = $child($source, 'exclude');
    foreach ($options['include'] as $path) {
        $path = '/workspace/' . $path;
        if (is_file($path)) {
            $include->appendChild($xml->createElement('file'))->textContent = $path;
            continue;
        }
        foreach ($plan['coverage_extensions'] as $extension) {
            $node = $include->appendChild($xml->createElement('directory'));
            $node->textContent = $path;
            $node->setAttribute('suffix', $extension);
        }
        $iterator = new RecursiveIteratorIterator(new RecursiveDirectoryIterator($path, FilesystemIterator::SKIP_DOTS), RecursiveIteratorIterator::SELF_FIRST);
        foreach ($iterator as $entry) {
            if ($entry->isDir() && in_array($entry->getFilename(), $plan['coverage_exclude_directories'] ?? [], true)) {
                $exclude->appendChild($xml->createElement('directory'))->textContent = $entry->getPathname();
            }
        }
    }
}
foreach ($options['exclude'] as $path) {
    $path = '/workspace/' . $path;
    $child($source, 'exclude')->appendChild($xml->createElement(is_dir($path) ? 'directory' : 'file'))->textContent = $path;
}
if (!$source->getElementsByTagName('include')->length) {
    throw new RuntimeException('Coverage needs a native source filter or tests.coverage.include.');
}
$output = tempnam(dirname($filename), '.phpwarden-coverage-');
if ($output === false || $xml->save($output) === false) {
    throw new RuntimeException('Could not prepare isolated coverage configuration.');
}
echo $output;
