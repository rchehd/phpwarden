<?php
function phpwarden_expensive_lookup(): int {
    $value = str_repeat('x', 1024);
    for ($i = 0; $i < 2500; $i++) {
        $value = hash('sha256', $value . str_repeat('y', 1024));
    }
    return strlen($value);
}
function phpwarden_render_item(): int {
    return phpwarden_expensive_lookup();
}
function phpwarden_scenario(): int {
    $total = 0;
    $cached = null;
    for ($i = 0; $i < 24; $i++) {
        if (getenv('SPIKE_VARIANT') === 'cached') {
            $cached ??= phpwarden_render_item();
            $total += $cached;
        } else {
            $total += phpwarden_render_item();
        }
    }
    return $total;
}
echo json_encode(['php' => PHP_VERSION, 'value' => phpwarden_scenario()]), "\n";
