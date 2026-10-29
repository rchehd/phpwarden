<?php

spl_autoload_register(static function (string $class): void {
    if ($class === 'PHPWarden\\PsalmDrupal\\Plugin') {
        require_once __DIR__ . '/Plugin.php';
    }
});

require (getenv('PHPWARDEN_PSALM_BUNDLE') ?: '/tool-vendor') . '/bin/psalm';
