<?php

namespace PHPWarden\Psalm\Drupal;

use RecursiveCallbackFilterIterator;
use RecursiveDirectoryIterator;
use RecursiveIteratorIterator;
use RuntimeException;
use Symfony\Component\Yaml\Yaml;

final class Input
{
    public array $hashes = [];
    public array $diagnostics = [];
    public readonly string $root;

    public function __construct(public readonly array $request)
    {
        $this->root = realpath($request['root']) ?: throw new RuntimeException('Project root missing');
        foreach (['composer_root', 'web_root', 'source_roots', 'include', 'exclude'] as $field) {
            if (!array_key_exists($field, $request)) {
                throw new RuntimeException('Missing preparation field: ' . $field);
            }
        }
    }

    public function path(string $relative): string
    {
        if ($relative === '' || str_starts_with($relative, '/') || in_array('..', explode('/', $relative), true)) {
            throw new RuntimeException('Expected project-relative path: ' . $relative);
        }
        $path = realpath($this->root . '/' . $relative);
        if ($path === false || ($path !== $this->root && !str_starts_with($path, $this->root . '/'))) {
            throw new RuntimeException('Missing or external project path: ' . $relative);
        }
        return $path;
    }

    public function relative(string $absolute): string
    {
        return ltrim(substr($absolute, strlen($this->root)), '/');
    }

    public function read(string $relative): string
    {
        $text = file_get_contents($this->path($relative));
        if ($text === false) {
            throw new RuntimeException('Cannot read ' . $relative);
        }
        $this->hashes[$relative] = hash('sha256', $text);
        return $text;
    }

    public function yaml(string $relative): array
    {
        $value = Yaml::parse($this->read($relative), Yaml::PARSE_CUSTOM_TAGS);
        if ($value !== null && !is_array($value)) {
            throw new RuntimeException('Expected YAML mapping: ' . $relative);
        }
        return $value ?? [];
    }

    public function diagnostic(string $code, string $file, string $detail): void
    {
        $diagnostic = compact('code', 'file', 'detail');
        if (!in_array($diagnostic, $this->diagnostics, true)) {
            $this->diagnostics[] = $diagnostic;
        }
    }

    public function matches(string $path, array $patterns): bool
    {
        foreach ($patterns as $pattern) {
            $pattern = rtrim($pattern, '/');
            if ($pattern === '.' || $path === $pattern || str_starts_with($path, $pattern . '/')) {
                return true;
            }
        }
        return false;
    }

    public function excluded(string $path): bool
    {
        foreach ($this->request['exclude'] as $pattern) {
            $patterns = explode('/', $pattern);
            $files = explode('/', $path);
            $cache = [];
            $match = function (int $p, int $f) use (&$match, &$cache, $patterns, $files): bool {
                $key = $p . ':' . $f;
                if (array_key_exists($key, $cache)) {
                    return $cache[$key];
                }
                if ($p === count($patterns)) {
                    return $cache[$key] = $f === count($files);
                }
                if ($patterns[$p] === '**') {
                    return $cache[$key] = $match($p + 1, $f) || ($f < count($files) && $match($p, $f + 1));
                }
                $segment = str_replace('[^', '[!', $patterns[$p]);
                return $cache[$key] = $f < count($files) && fnmatch($segment, $files[$f]) && $match($p + 1, $f + 1);
            };
            if ($match(0, 0)) {
                return true;
            }
        }
        return false;
    }

    public function files(string $directory): array
    {
        if (isset($this->request['inventory'])) {
            $prefix = $this->relative($this->path($directory));
            return array_values(array_filter($this->request['inventory'], fn ($name) => $name === $prefix || str_starts_with($name, $prefix . '/')));
        }
        $iterator = new RecursiveDirectoryIterator($this->path($directory), \FilesystemIterator::SKIP_DOTS);
        $filter = new RecursiveCallbackFilterIterator($iterator, function ($file): bool {
            $relative = $this->relative($file->getPathname());
            if (!$file->isDir() && $this->excluded($relative)) {
                return false;
            }
            if ($file->isLink()) {
                if ($this->excluded($relative)) {
                    return false;
                }
                $this->diagnostic('symlink', $relative, 'Source symlinks require explicit resolution');
                return false;
            }
            return !$file->isDir() || !in_array($file->getFilename(), ['tests', 'vendor', 'node_modules', 'files', '.git'], true);
        });
        $files = [];
        foreach (new RecursiveIteratorIterator($filter) as $file) {
            if ($file->isFile()) {
                $files[] = $this->relative($file->getPathname());
            }
        }
        sort($files);
        return $files;
    }

    public function extensions(): array
    {
        $extensions = [];
        foreach ($this->request['source_roots'] as $directory) {
            foreach ($this->files($directory) as $file) {
                if (!preg_match('~(?:^|/)([a-z][a-z0-9_]*)\.info\.yml$~', $file, $match)) {
                    continue;
                }
                $metadata = $this->yaml($file);
                $type = $metadata['type'] ?? null;
                if (!in_array($type, ['module', 'theme', 'profile'], true)) {
                    throw new RuntimeException('Unsupported extension type: ' . $file);
                }
                $path = dirname($file);
                if ($this->request['include'] && !$this->matches($path, $this->request['include'])) {
                    $selected = array_filter($this->files($path), fn ($name) => $this->matches($name, $this->request['include']));
                    if (!$selected) {
                        continue;
                    }
                }
                if (isset($extensions[$match[1]]) && $extensions[$match[1]]['path'] !== $path) {
                    throw new RuntimeException('Ambiguous extension name: ' . $match[1]);
                }
                $extensions[$match[1]] = ['name' => $match[1], 'type' => $type, 'path' => $path];
            }
        }
        ksort($extensions);
        return array_values($extensions);
    }
}
