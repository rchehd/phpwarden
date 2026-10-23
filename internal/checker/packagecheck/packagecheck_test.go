package packagecheck

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings(t *testing.T) config.Config {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := effective.Config
	cfg.Tools.ComposerRequireChecker.Enabled = true
	cfg.Tools.ComposerUnused.Enabled = true
	return cfg
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if os.MkdirAll(filepath.Dir(filename), 0o700) != nil {
		t.Fatal("mkdir")
	}
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "composer.json", `{"name":"demo/root","version":"1.0.0","require":{"php":">=8.1","acme/a":"^1","acme/b":"^1"},"autoload":{"psr-4":{"Demo\\":"src/"}}}`)
	write(t, root, "src/Demo.php", "<?php\nnamespace Demo;\nclass Demo { public function go(): void { new \\Acme\\A\\Used(); } }\n")
	packages := []map[string]any{}
	for _, pkg := range []string{"a", "b"} {
		namespace := "Acme\\" + strings.ToUpper(pkg) + "\\"
		class := "Used"
		if pkg == "b" {
			class = "Unused"
		}
		data, _ := json.Marshal(map[string]any{"name": "acme/" + pkg, "version": "1.0.0", "type": "library", "require": map[string]string{"php": ">=8.1"}, "autoload": map[string]any{"psr-4": map[string]string{namespace: "src/"}}})
		write(t, root, "vendor/acme/"+pkg+"/composer.json", string(data))
		write(t, root, "vendor/acme/"+pkg+"/src/"+class+".php", "<?php\nnamespace "+strings.TrimSuffix(namespace, "\\")+";\nclass "+class+" {}\n")
		var entry map[string]any
		json.Unmarshal(data, &entry)
		entry["install-path"] = "../acme/" + pkg
		entry["source"] = map[string]string{"type": "git", "url": "https://example.com/acme/" + pkg, "reference": "fixture"}
		packages = append(packages, entry)
	}
	data, _ := json.Marshal(map[string]any{"packages": packages, "dev": true, "dev-package-names": []string{}})
	write(t, root, "vendor/composer/installed.json", string(data))
	write(t, root, "vendor/composer/installed.php", `<?php return ['root'=>['name'=>'demo/root','version'=>'1.0.0.0','pretty_version'=>'1.0.0','type'=>'library','install_path'=>__DIR__.'/../..','aliases'=>[],'dev'=>true], 'versions'=>['acme/a'=>['pretty_version'=>'1.0.0','version'=>'1.0.0.0','reference'=>'fixture','type'=>'library','install_path'=>__DIR__.'/../acme/a','aliases'=>[],'dev_requirement'=>false], 'acme/b'=>['pretty_version'=>'1.0.0','version'=>'1.0.0.0','reference'=>'fixture','type'=>'library','install_path'=>__DIR__.'/../acme/b','aliases'=>[],'dev_requirement'=>false]]];`)
	return root
}

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func TestNativeDependencyChecksAndUnknownSymbols(t *testing.T) {
	root, cfg := fixture(t), settings(t)
	for _, id := range []string{"composer-require-checker", "composer-unused"} {
		bundle, _ := filepath.Abs("../../../tools/" + id)
		if id == "composer-require-checker" {
			bundle = filepath.Join(bundle, "php8.4")
		}
		result := run(context.Background(), root, scope.Selection{Mode: "full"}, cfg, id, bundle, runner.Exec{})
		if result.Check.Status != "passed" || len(result.Diagnostics) != 0 {
			t.Fatalf("%s native result: %+v", id, result)
		}
		if id == "composer-unused" && (len(result.Findings) != 1 || !strings.Contains(result.Findings[0].Message, "acme/b")) {
			t.Fatalf("unused candidate: %+v", result)
		}
	}
	write(t, root, "src/Demo.php", "<?php\nnamespace Demo;\nclass Demo extends \\Undeclared\\Missing {}\n")
	bundle, _ := filepath.Abs("../../../tools/composer-require-checker/php8.4")
	result := run(context.Background(), root, scope.Selection{Mode: "full"}, cfg, "composer-require-checker", bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 1 || !strings.Contains(result.Findings[0].Message, `Undeclared\Missing`) {
		t.Fatalf("unknown symbol: %+v", result)
	}
	cfg.Tools.ComposerRequireChecker.AllowedSymbols = []string{`Undeclared\Missing`}
	result = run(context.Background(), root, scope.Selection{Mode: "full"}, cfg, "composer-require-checker", bundle, runner.Exec{})
	if len(result.Findings) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("symbol whitelist: %+v", result)
	}
	cfg.Tools.ComposerRequireChecker.AllowedSymbols = []string{}
	write(t, root, "composer-require-checker.json", `{"symbol-whitelist":["Undeclared\\Missing"]}`)
	result = run(context.Background(), root, scope.Selection{Mode: "full"}, cfg, "composer-require-checker", bundle, runner.Exec{})
	if len(result.Findings) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("native JSON config: %+v", result)
	}
	write(t, root, "src/Demo.php", "<?php\nnamespace Demo;\nclass Demo { public function go(): void { new \\Acme\\A\\Used(); } }\n")
	write(t, root, "composer-unused.php", `<?php return static function ($config) { return $config->addNamedFilter(\ComposerUnused\ComposerUnused\Configuration\NamedFilter::fromString('acme/b')); };`)
	bundle, _ = filepath.Abs("../../../tools/composer-unused")
	result = run(context.Background(), root, scope.Selection{Mode: "full"}, cfg, "composer-unused", bundle, runner.Exec{})
	if len(result.Findings) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("native PHP config: %+v", result)
	}
}

func TestMissingDependenciesUnsupportedLayoutAndFullOnly(t *testing.T) {
	root, cfg := fixture(t), settings(t)
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected native process")
		return runner.Result{}
	})
	for _, id := range []string{"composer-require-checker", "composer-unused"} {
		result := run(context.Background(), root, scope.Selection{Mode: "diff"}, cfg, id, "", unused)
		if *result.Check.Reason != "full_scan_required" {
			t.Fatalf("diff scan ran: %+v", result)
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, "vendor/composer/installed.json"))
	write(t, root, "vendor/composer/installed.json", strings.Replace(string(data), "../acme/a", "../../web/core", 1))
	for _, id := range []string{"composer-require-checker", "composer-unused"} {
		_, reason, err := prepare(root, cfg, id)
		if err != nil || reason != "unsupported_layout" {
			t.Fatalf("Drupal installer layout accepted: %s %v", reason, err)
		}
	}
	if os.Remove(filepath.Join(root, "vendor/composer/installed.json")) != nil {
		t.Fatal("remove")
	}
	_, reason, err := prepare(root, cfg, "composer-unused")
	if err == nil || reason != "dependencies_missing" {
		t.Fatal("missing metadata passed")
	}
}

func TestPackageReportCompletenessAndFailures(t *testing.T) {
	root, cfg := fixture(t), settings(t)
	bundle := t.TempDir()
	write(t, bundle, "vendor/autoload.php", "<?php")
	valid := map[string]any{"version": 1, "tool_version": UnusedVersion, "runtime": "8.4", "php_version": "8.4.26", "status": 1, "data": map[string]any{"used-packages": []map[string]string{{"name": "php"}, {"name": "acme/a"}}, "unused-packages": []string{"acme/b"}}}
	for _, name := range []string{"valid", "partial", "version", "stderr", "timeout", "malformed", "missing PHP"} {
		t.Run(name, func(t *testing.T) {
			data, _ := json.Marshal(valid)
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if !slices.Contains(cmd.Args, "memory_limit=1024M") {
					t.Fatal("memory limit missing")
				}
				r := runner.Result{Stdout: data}
				switch name {
				case "partial":
					r.Stdout = []byte(strings.Replace(string(data), `{"name":"acme/a"}`, `{"name":"acme/c"}`, 1))
				case "version":
					r.Stdout = []byte(strings.Replace(string(data), UnusedVersion, "0.0.0", 1))
				case "stderr":
					r.Stderr = []byte("warning")
				case "timeout":
					r.TimedOut = true
				case "malformed":
					r.Stdout = []byte("{")
				case "missing PHP":
					r.Unavailable = true
				}
				return r
			})
			result := run(context.Background(), root, scope.Selection{Mode: "full"}, cfg, "composer-unused", bundle, process)
			if name == "valid" {
				if len(result.Findings) != 1 || result.Check.Status != "passed" {
					t.Fatalf("valid report rejected: %+v", result)
				}
			} else if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Findings) > 0 {
				t.Fatalf("failure accepted: %+v", result)
			}
		})
	}
}
