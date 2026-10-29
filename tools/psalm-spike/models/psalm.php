<?php

spl_autoload_register(static function (string $class): void {
    if ($class === 'PHPWarden\\PsalmSpike\\Plugin') {
        require_once __DIR__ . '/Plugin.php';
    }
});

require '/tool-vendor/bin/psalm';
