<?php

namespace PHPWarden\Psalm\Drupal;

final class Services
{
    public array $types = [];
    public array $unresolved = [];
    private array $definitions = [];
    private array $parameters = [];

    public function __construct(private readonly Input $input, array $files)
    {
        foreach (array_unique($files) as $file) {
            $document = $input->yaml($file);
            foreach ($document['imports'] ?? [] as $import) {
                $input->diagnostic('service-import', $file, 'Service imports are not resolved by this candidate');
            }
            foreach ($document['parameters'] ?? [] as $id => $value) {
                if (isset($this->parameters[$id]) && $this->parameters[$id] !== $value) {
                    $input->diagnostic('parameter-conflict', $file, (string) $id);
                }
                $this->parameters[$id] = $value;
            }
            foreach ($document['services'] ?? [] as $id => $definition) {
                if (str_starts_with((string) $id, '_')) {
                    continue;
                }
                if (isset($this->definitions[$id]) && $this->definitions[$id]['value'] !== $definition) {
                    $this->definitions[$id] = ['value' => null, 'file' => $file];
                    $this->unresolved[$id] = 'Conflicting definitions without an enabled-site container';
                    continue;
                }
                $this->definitions[$id] = ['value' => $definition, 'file' => $file];
            }
        }
        foreach ($this->definitions as $id => $definition) {
            if (is_array($definition['value']) && isset($definition['value']['decorates'])) {
                $target = $definition['value']['decorates'];
                $this->unresolved[$id] = 'Service decoration requires a compiled container';
                if (is_string($target)) {
                    $this->unresolved[$target] = 'Decorated service requires a compiled container';
                }
            }
        }
        foreach (array_keys($this->definitions) as $id) {
            $this->resolve((string) $id, []);
        }
        ksort($this->types);
        ksort($this->unresolved);
    }

    private function resolve(string $id, array $seen): ?string
    {
        if (isset($this->types[$id])) {
            return $this->types[$id];
        }
        if (isset($this->unresolved[$id])) {
            return null;
        }
        if (isset($seen[$id]) || !isset($this->definitions[$id])) {
            $this->unresolved[$id] = isset($seen[$id]) ? 'Alias cycle' : 'Unknown service';
            return null;
        }
        $seen[$id] = true;
        $definition = $this->definitions[$id]['value'];
        $alias = is_string($definition) && str_starts_with($definition, '@') ? substr($definition, 1) : null;
        if (is_array($definition)) {
            $alias = $definition['alias'] ?? $alias;
        }
        if (is_string($alias)) {
            $type = $this->resolve($alias, $seen);
        } elseif (is_array($definition) && !isset($definition['decorates'])) {
            $type = $definition['class'] ?? null;
            if (is_string($type) && preg_match('/^%([^%]+)%$/', $type, $match)) {
                $type = $this->parameters[$match[1]] ?? null;
            }
        } else {
            $type = null;
        }
        if (!is_string($type) || !preg_match('/^\\\\?[A-Za-z_][A-Za-z0-9_]*(?:\\\\[A-Za-z_][A-Za-z0-9_]*)*$/', $type)) {
            $this->unresolved[$id] ??= 'No unambiguous literal class or resolvable alias';
            return null;
        }
        return $this->types[$id] = ltrim($type, '\\');
    }

    public function used(string $id, string $file): ?string
    {
        if (!isset($this->types[$id])) {
            $this->input->diagnostic('service-unresolved', $file, $id . ': ' . ($this->unresolved[$id] ?? 'Unknown service'));
        }
        return $this->types[$id] ?? null;
    }

    public function xml(): string
    {
        $document = new \DOMDocument('1.0', 'UTF-8');
        $container = $document->appendChild($document->createElement('container'));
        $container->setAttribute('xmlns', 'http://symfony.com/schema/dic/services');
        $services = $container->appendChild($document->createElement('services'));
        foreach ($this->types as $id => $type) {
            $service = $services->appendChild($document->createElement('service'));
            $service->setAttribute('id', (string) $id);
            $service->setAttribute('class', $type);
        }
        return $document->saveXML();
    }
}
