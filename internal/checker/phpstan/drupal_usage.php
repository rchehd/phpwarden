<?php

declare(strict_types=1);

use ShipMonk\PHPStan\DeadCode\Provider\ReflectionBasedMemberUsageProvider;
use ShipMonk\PHPStan\DeadCode\Provider\VirtualUsageData;
use Symfony\Component\Yaml\Yaml;

// Marks members that Drupal calls without a PHP call site: hooks, container-built constructors, string callbacks and YAML routes.
final class PHPWardenDrupalUsageProvider extends ReflectionBasedMemberUsageProvider
{
    private const ATTRIBUTES = ['Drupal\\Core\\Hook\\Attribute\\', 'Drush\\Attributes\\'];

    private const CALLBACK_INTERFACES = ['Drupal\\Core\\Security\\TrustedCallbackInterface', 'Symfony\\Component\\EventDispatcher\\EventSubscriberInterface'];

    /** @var array<string, array<string, true>> */
    private array $fileReferences = [];

    /** @var array<string, array<string, array<string, true>>> */
    private array $yamlReferences = [];

    public function shouldMarkMethodAsUsed(ReflectionMethod $method): ?VirtualUsageData
    {
        if ($method->isConstructor()) {
            return VirtualUsageData::withNote('Drupal builds services, plugins, forms and controllers');
        }
        foreach ($method->getAttributes() as $attribute) {
            if ($this->drupalAttribute($attribute->getName())) {
                return VirtualUsageData::withNote('Drupal or Drush attribute');
            }
        }
        $class = $method->getDeclaringClass();
        foreach ($class->getAttributes() as $attribute) {
            if ($this->drupalAttribute($attribute->getName())) {
                $arguments = $attribute->getArguments();
                if (($arguments['method'] ?? $arguments[1] ?? '__invoke') === $method->getName()) {
                    return VirtualUsageData::withNote('Drupal hook attribute on the class');
                }
            }
        }
        $file = $class->getFileName();
        if ($file === false) {
            return null;
        }
        $name = strtolower($method->getName());
        $references = $this->fileReferences($file, $this->callbackClass($class));
        if (isset($references[$name])) {
            return VirtualUsageData::withNote('String callback in the same class');
        }
        if (isset($this->yamlReferences($file)[strtolower(ltrim($class->getName(), '\\'))][$name])) {
            return VirtualUsageData::withNote('Referenced from module YAML');
        }
        return null;
    }

    private function drupalAttribute(string $name): bool
    {
        foreach (self::ATTRIBUTES as $prefix) {
            if (str_starts_with($name, $prefix)) {
                return true;
            }
        }
        return false;
    }

    private function callbackClass(ReflectionClass $class): bool
    {
        foreach (self::CALLBACK_INTERFACES as $interface) {
            if (interface_exists($interface) && $class->implementsInterface($interface)) {
                return true;
            }
        }
        return false;
    }

    /** @return array<string, true> */
    private function fileReferences(string $file, bool $anyString): array
    {
        $key = $file . ($anyString ? '#any' : '');
        if (!isset($this->fileReferences[$key])) {
            $source = (string) @file_get_contents($file);
            $patterns = ['/[\'"]::(\w+)[\'"]/', '/\[\s*(?:\$this|static::class|self::class|static|self)\s*,\s*[\'"](\w+)[\'"]\s*\]/'];
            if ($anyString) {
                $patterns[] = '/[\'"](\w+)[\'"]/';
            }
            $names = [];
            foreach ($patterns as $pattern) {
                preg_match_all($pattern, $source, $matches);
                foreach ($matches[1] as $match) {
                    $names[strtolower($match)] = true;
                }
            }
            $this->fileReferences[$key] = $names;
        }
        return $this->fileReferences[$key];
    }

    /** @return array<string, array<string, true>> class => methods referenced from the module's routing and services YAML */
    private function yamlReferences(string $file): array
    {
        $module = dirname($file);
        while (glob($module . '/*.info.yml') === [] && dirname($module) !== $module) {
            $module = dirname($module);
        }
        if (!isset($this->yamlReferences[$module])) {
            $services = [];
            $values = [];
            foreach (array_merge(glob($module . '/*.services.yml') ?: [], glob($module . '/*.routing.yml') ?: []) as $yaml) {
                try {
                    $data = Yaml::parseFile($yaml);
                } catch (Throwable) {
                    continue;
                }
                if (str_ends_with($yaml, '.services.yml')) {
                    foreach ($data['services'] ?? [] as $id => $service) {
                        if (is_array($service) && is_string($service['class'] ?? null)) {
                            $services[$id] = $service['class'];
                        } elseif (is_array($service) && class_exists((string) $id)) {
                            $services[$id] = $id;
                        }
                    }
                }
                array_walk_recursive($data, static function ($value) use (&$values): void {
                    if (is_string($value)) {
                        $values[] = $value;
                    }
                });
            }
            $references = [];
            foreach ($values as $value) {
                if (preg_match('/^\\\\?([\w\\\\]+)::(\w+)$/', $value, $match)) {
                    $references[strtolower($match[1])][strtolower($match[2])] = true;
                } elseif (preg_match('/^([\w.]+):(\w+)$/', $value, $match) && isset($services[$match[1]])) {
                    $references[strtolower(ltrim($services[$match[1]], '\\'))][strtolower($match[2])] = true;
                }
            }
            $this->yamlReferences[$module] = $references;
        }
        return $this->yamlReferences[$module];
    }
}
