<?php

libxml_use_internal_errors(true);
$errors = [];
$checked = 0;
try {
    $files = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    if (!is_array($files) || count($files) !== 2 || !is_file($argv[1] ?? '')) {
        throw new RuntimeException('Expected two configurations and the pinned Psalm schema');
    }
    foreach ($files as $file) {
        $document = new DOMDocument();
        libxml_clear_errors();
        if (!$document->load($file, LIBXML_NONET) || $document->doctype !== null) {
            throw new RuntimeException('Invalid prepared XML');
        }
        $document->documentElement->setAttribute('xmlns', 'https://getpsalm.org/schema/config');
        $normalized = new DOMDocument();
        if (!$normalized->loadXML($document->saveXML(), LIBXML_NONET) || !$normalized->schemaValidate($argv[1])) {
            $before = count($errors);
            foreach (libxml_get_errors() as $error) {
                $errors[] = trim($error->message) . ' (line ' . $error->line . ')';
            }
            if (count($errors) === $before) {
                $errors[] = 'Prepared configuration failed schema validation';
            }
        }
        ++$checked;
    }
} catch (Throwable $error) {
    $errors[] = $error->getMessage();
}
echo json_encode(['version' => 1, 'checked' => $checked, 'errors' => $errors], JSON_THROW_ON_ERROR) . "\n";
exit($errors ? 3 : 0);
