<?php

$files = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
$errors = [];
foreach ($files as $file) {
    try {
        $source = file_get_contents($file);
        if ($source === false) {
            throw new RuntimeException('Cannot read source');
        }
        token_get_all($source, TOKEN_PARSE);
    } catch (Throwable $error) {
        $errors[] = ['file' => $file, 'message' => $error->getMessage()];
    }
}
echo json_encode(['version' => 1, 'checked' => count($files), 'errors' => $errors], JSON_THROW_ON_ERROR);
exit($errors ? 3 : 0);
