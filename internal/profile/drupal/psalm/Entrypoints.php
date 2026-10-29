<?php

namespace PHPWarden\Psalm\Drupal;

use PhpParser\Node;
use PhpParser\NodeFinder;
use PhpParser\NodeTraverser;
use PhpParser\NodeVisitor\NameResolver;
use PhpParser\ParserFactory;

final class Entrypoints
{
    public array $classes = [];
    public array $routes = [];
    public array $sources = [];
    public string $calls = "<?php\nnamespace PHPWarden\\Generated\\Psalm;\n";
    public string $stubs = "<?php\nnamespace PHPWarden\\Generated\\Psalm;\n";
    private int $sequence = 0;

    public function __construct(private readonly Input $input, private readonly Services $services, array $files)
    {
        $parser = (new ParserFactory())->createForNewestSupportedVersion();
        $finder = new NodeFinder();
        foreach ($files as $file) {
            if (!preg_match('/\.(php|module|theme|inc|install|profile|engine|test)$/', $file)) {
                continue;
            }
            if ($input->request['include'] && !$input->matches($file, $input->request['include'])) {
                continue;
            }
            $this->sources[] = $file;
            $nodes = (new NodeTraverser(new NameResolver()))->traverse($parser->parse($input->read($file)) ?? []);
            foreach ($finder->findInstanceOf($nodes, Node\Stmt\Class_::class) as $class) {
                if (!$class->name) {
                    continue;
                }
                $name = (string) $class->namespacedName;
                if (isset($this->classes[$name])) {
                    $input->diagnostic('class-conflict', $file, $name);
                    continue;
                }
                $this->classes[$name] = ['node' => $class, 'file' => $file];
                if (str_ends_with($name, 'ServiceProvider')) {
                    $input->diagnostic('dynamic-container', $file, 'ServiceProvider changes are not executed');
                }
                $queueInterface = array_filter($class->implements, fn ($name) => (string) $name === 'Drupal\\Core\\Queue\\QueueWorkerInterface');
                if (($class->extends && str_ends_with((string) $class->extends, '\\QueueWorkerBase')) || $queueInterface) {
                    $input->diagnostic('queue-entrypoint', $file, 'Queue producers and persistence are not modeled');
                }
            }
            foreach ($finder->findInstanceOf($nodes, Node\Expr\StaticCall::class) as $call) {
                if (
                    !$call->class instanceof Node\Name || (string) $call->class !== 'Drupal'
                    || !$call->name instanceof Node\Identifier || (string) $call->name !== 'service'
                ) {
                    continue;
                }
                $argument = $call->getArgs()[0]->value ?? null;
                if ($argument instanceof Node\Scalar\String_) {
                    $services->used($argument->value, $file);
                } elseif (
                    $argument instanceof Node\Expr\ClassConstFetch && $argument->class instanceof Node\Name
                    && $argument->name instanceof Node\Identifier && (string) $argument->name === 'class'
                ) {
                    $services->used((string) $argument->class, $file);
                } else {
                    $input->diagnostic('dynamic-service', $file, 'Nonliteral Drupal::service lookup');
                }
            }
        }
    }

    private function instance(string $class): string
    {
        $function = 'phpwarden_psalm_instance_' . ++$this->sequence;
        $this->stubs .= 'function ' . $function . '(): \\' . ltrim($class, '\\') . " {}\n";
        return $function . '()';
    }

    private function arguments(Node\Stmt\ClassMethod $method, array $route, string $file): ?string
    {
        $arguments = [];
        preg_match_all('/\{([A-Za-z_][A-Za-z0-9_]*)\}/', $route['path'] ?? '', $matches);
        foreach ($method->params as $parameter) {
            $name = $parameter->var->name;
            if (!is_string($name) || $parameter->variadic || $parameter->byRef) {
                $this->input->diagnostic('route-parameter', $file, 'Unsupported parameter declaration');
                return null;
            }
            $value = null;
            if (in_array($name, $matches[1], true)) {
                if ($parameter->type !== null && !$parameter->type instanceof Node\Identifier) {
                    $this->input->diagnostic('route-conversion', $file, $name . ': ' . $parameter->type->getType());
                    return null;
                }
                $type = $parameter->type === null ? 'string' : (string) $parameter->type;
                if (!in_array($type, ['string', 'int', 'float', 'bool', 'mixed'], true)) {
                    $this->input->diagnostic('route-conversion', $file, $name . ': ' . $type);
                    return null;
                }
                $value = '(' . ($type === 'mixed' ? 'string' : $type) . ') $_GET[' . var_export($name, true) . ']';
            } elseif (
                array_key_exists($name, $route['defaults'] ?? [])
                && (is_scalar($route['defaults'][$name]) || $route['defaults'][$name] === null)
            ) {
                $value = var_export($route['defaults'][$name], true);
            } elseif ($parameter->type instanceof Node\Name && (string) $parameter->type === 'Symfony\\Component\\HttpFoundation\\Request') {
                $value = $this->instance((string) $parameter->type);
            } elseif ($parameter->default !== null) {
                continue;
            } else {
                $this->input->diagnostic('route-parameter', $file, 'Cannot resolve argument ' . $name);
                return null;
            }
            $arguments[] = $name . ': ' . $value;
        }
        return implode(', ', $arguments);
    }

    public function routing(string $file): void
    {
        foreach ($this->input->yaml($file) as $id => $route) {
            if (!is_array($route) || !isset($route['defaults']) || !is_array($route['defaults'])) {
                $this->input->diagnostic('route-definition', $file, (string) $id);
                continue;
            }
            $defaults = $route['defaults'];
            $record = ['id' => (string) $id, 'file' => $file, 'modeled' => false];
            $before = count($this->input->diagnostics);
            if (isset($defaults['_controller']) && is_string($defaults['_controller'])) {
                $target = $defaults['_controller'];
                if (str_contains($target, '::')) {
                    [$class, $method] = explode('::', $target, 2);
                    $class = ltrim($class, '\\');
                    if (isset($this->services->types[$class]) || isset($this->services->unresolved[$class])) {
                        $class = $this->services->used($class, $file);
                    }
                } elseif (str_contains($target, ':')) {
                    [$service, $method] = explode(':', $target, 2);
                    $class = $this->services->used($service, $file);
                } else {
                    $class = ltrim($target, '\\');
                    $method = '__invoke';
                }
                if ($class !== null && isset($this->classes[$class]) && preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $method)) {
                    $node = $this->classes[$class]['node']->getMethod($method);
                    if ($node !== null && $node->isPublic()) {
                        $arguments = $this->arguments($node, $route, $file);
                        if ($arguments !== null) {
                            $this->calls .= '$controller = ' . $this->instance($class) . ";\n";
                            $array = $node->returnType instanceof Node\Identifier && (string) $node->returnType === 'array';
                            $this->calls .= ($array ? '$build = ' : '') . '$controller->' . $method . '(' . $arguments . ");\n";
                            if ($node->returnType === null) {
                                $this->input->diagnostic('route-return', $file, 'Untyped callback render result: ' . $id);
                            }
                            if ($node->returnType instanceof Node\Identifier && (string) $node->returnType === 'array') {
                                $this->services->used('renderer', $file);
                                $this->calls .= '\\Drupal::service("renderer")->render($build);' . "\n";
                            }
                            $record['modeled'] = true;
                        }
                    }
                }
            } elseif (isset($defaults['_form']) && is_string($defaults['_form'])) {
                $class = ltrim($defaults['_form'], '\\');
                if (isset($this->classes[$class])) {
                    $this->calls .= '$form = ' . $this->instance($class) . ";\n";
                    $this->calls .= '$form_state = ' . $this->instance('Drupal\\Core\\Form\\FormState') . ";\n";
                    $this->calls .= '$build = $form->buildForm([], $form_state);' . "\n";
                    $this->services->used('renderer', $file);
                    $this->calls .= '\\Drupal::service("renderer")->render($build);' . "\n";
                    $record['modeled'] = true;
                }
            }
            if (!$record['modeled'] && count($this->input->diagnostics) === $before) {
                $this->input->diagnostic('route-target', $file, 'Unsupported or unresolved callback: ' . $id);
            }
            $this->routes[] = $record;
        }
    }
}
