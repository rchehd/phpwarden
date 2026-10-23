<?php

declare(strict_types=1);

function significant(array $tokens, int $index, int $direction): mixed
{
    for ($i = $index + $direction; isset($tokens[$i]); $i += $direction) {
        $token = $tokens[$i];
        if (!is_array($token) || !in_array($token[0], [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT], true)) {
            return [$token, $i];
        }
    }
    return [null, -1];
}

// Returns the significant tokens of the call's second top-level argument, or null without one.
function secondArgument(array $tokens, int $open): ?array
{
    $depth = 0;
    $argument = 0;
    $result = [];
    for ($i = $open; isset($tokens[$i]); $i++) {
        $token = $tokens[$i];
        if (in_array($token, ['(', '[', '{'], true) || is_array($token) && in_array($token[0], [T_CURLY_OPEN, T_DOLLAR_OPEN_CURLY_BRACES], true)) {
            $depth++;
        } elseif (in_array($token, [')', ']', '}'], true)) {
            $depth--;
            if ($depth === 0) {
                break;
            }
        } elseif ($token === ',' && $depth === 1) {
            $argument++;
            continue;
        }
        if ($argument === 1 && $depth >= 1 && !(is_array($token) && in_array($token[0], [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT], true))) {
            $result[] = is_array($token) ? strtolower($token[1]) : $token;
        }
    }
    return $argument === 0 || $result === [] ? null : $result;
}

try {
    $request = json_decode(stream_get_contents(STDIN), true, 512, JSON_THROW_ON_ERROR);
    $functions = array_fill_keys(array_map('strtolower', $request['functions']), true);
    $findings = [];
    foreach ($request['files'] as $path) {
        $source = file_get_contents($path);
        if ($source === false) {
            throw new RuntimeException('Cannot read ' . $path);
        }
        $tokens = token_get_all($source);
        $attributeDepth = 0;
        foreach ($tokens as $index => $token) {
            if (is_array($token) && $token[0] === T_ATTRIBUTE) {
                $attributeDepth++;
                continue;
            }
            if ($attributeDepth > 0) {
                if ($token === '[') {
                    $attributeDepth++;
                } elseif ($token === ']') {
                    $attributeDepth--;
                }
                continue;
            }
            if (is_array($token) && $token[0] === T_EXIT && isset($functions[strtolower($token[1])])) {
                // Without TOKEN_PARSE, Demo::die() and function die() still lex die as T_EXIT.
                [$previous] = significant($tokens, $index, -1);
                if (!is_array($previous) || !in_array($previous[0], [T_FUNCTION, T_OBJECT_OPERATOR, T_NULLSAFE_OBJECT_OPERATOR, T_DOUBLE_COLON, T_CONST], true)) {
                    $findings[] = ['path' => $path, 'line' => $token[2], 'function' => strtolower($token[1])];
                }
                continue;
            }
            if (!is_array($token) || !in_array($token[0], [T_STRING, T_NAME_FULLY_QUALIFIED, T_NAME_QUALIFIED], true)) {
                continue;
            }
            $name = strtolower(ltrim($token[1], '\\'));
            if (!isset($functions[$name])) {
                continue;
            }
            [$next, $nextIndex] = significant($tokens, $index, 1);
            if ($next !== '(') {
                continue;
            }
            [$previous, $previousIndex] = significant($tokens, $index, -1);
            if ($previous === '&' || is_array($previous) && in_array($previous[0], [T_AMPERSAND_FOLLOWED_BY_VAR_OR_VARARG, T_AMPERSAND_NOT_FOLLOWED_BY_VAR_OR_VARARG], true)) {
                [$previous] = significant($tokens, $previousIndex, -1);
            }
            if (is_array($previous) && in_array($previous[0], [T_FUNCTION, T_NEW, T_OBJECT_OPERATOR, T_NULLSAFE_OBJECT_OPERATOR, T_DOUBLE_COLON], true)) {
                continue;
            }
            [$argument, $argumentIndex] = significant($tokens, $nextIndex, 1);
            if (is_array($argument) && $argument[0] === T_ELLIPSIS) {
                [$afterEllipsis] = significant($tokens, $argumentIndex, 1);
                if ($afterEllipsis === ')') {
                    continue;
                }
            }
            // print_r() with a truthy $return builds a string instead of printing.
            if ($name === 'print_r') {
                $return = secondArgument($tokens, $nextIndex);
                if ($return !== null && !in_array($return, [['false'], ['0'], ['null'], ['\\false'], ['\\null']], true)) {
                    continue;
                }
            }
            $findings[] = ['path' => $path, 'line' => $token[2], 'function' => $name];
        }
    }
    echo json_encode([
        'version' => 1,
        'runtime' => PHP_MAJOR_VERSION . '.' . PHP_MINOR_VERSION,
        'php_version' => PHP_VERSION,
        'findings' => $findings,
    ], JSON_THROW_ON_ERROR), "\n";
} catch (Throwable $error) {
    fwrite(STDERR, $error->getMessage() . "\n");
    exit(70);
}
