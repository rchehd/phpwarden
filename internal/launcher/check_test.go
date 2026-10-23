package launcher

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
	"github.com/rchehd/phpwarden/internal/worker"
)

type processFunc func(context.Context, runner.Command) runner.Result

func (process processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return process(ctx, command)
}

const testImage = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func fixture(t *testing.T) (config.Location, config.Effective, scope.Options) {
	t.Helper()
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	root := filepath.Join(t.TempDir(), "project, with spaces")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "demo.php"), []byte("<?php\nvar_dump(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, config.Filename)
	data := `{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},"scope":{"mode":"full","include":["src"]},"tools":{"syntax":{"enabled":false},"phpcs":{"enabled":false},"phpmd":{"enabled":false},"phpstan":{"enabled":false},"twig-cs-fixer":{"enabled":false}}}`
	if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", root, "init", "--quiet")
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	location := config.Location{Root: root, Path: filename}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	return location, effective, scope.Options{Root: root, CWD: root, Mode: "full", Include: []string{"src"}}
}

func dockerDouble(t *testing.T, location config.Location, calls *[]runner.Command) processFunc {
	t.Helper()
	return func(ctx context.Context, command runner.Command) runner.Result {
		*calls = append(*calls, command)
		if command.Name != "docker" {
			t.Fatalf("host must invoke only Docker: %+v", command)
		}
		switch command.Args[0] {
		case "context":
			return runner.Result{Stdout: []byte(`"unix:///var/run/docker.sock"`)}
		case "image":
			return runner.Result{Stdout: []byte(`[{"Id":"` + testImage + `","Os":"linux","Architecture":"amd64","Config":{"Labels":{"io.phpwarden.protocol":"2","io.phpwarden.toolset":"` + toolset.DevelopmentID("8.4") + `","io.phpwarden.checkers":"composer-audit,composer-validate,debug,drupal-config,phpcs,phpmd,phpstan,syntax,twig-cs-fixer,whitespace,yaml","io.phpwarden.php-version":"8.4.26","io.phpwarden.symfony-yaml-version":"6.4.48","io.phpwarden.phpcs-version":"4.0.4","io.phpwarden.coder-version":"9.0.1","io.phpwarden.phpcompatibility-version":"10.0.0-alpha2","io.phpwarden.phpstan-version":"2.3.1","io.phpwarden.phpstan-drupal-version":"2.2.2","io.phpwarden.phpstan-deprecations-version":"2.0.5","io.phpwarden.twig-cs-fixer-version":"4.1.1","io.phpwarden.twig-version":"3.30.0","io.phpwarden.composer-version":"2.10.3","io.phpwarden.phpmd-version":"3.0.0","io.phpwarden.pdepend-version":"3.0.0"}}}]`)}
		case "run":
			for _, argument := range []string{"--read-only", "--network=none", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--pull=never", testImage} {
				if !slices.Contains(command.Args, argument) {
					t.Fatalf("missing %s in Docker invocation: %v", argument, command.Args)
				}
			}
			mount := command.Args[slices.Index(command.Args, "--mount")+1]
			fields, err := csv.NewReader(strings.NewReader(mount)).Read()
			if err != nil || !reflect.DeepEqual(fields, []string{"type=bind", "source=" + location.Root, "target=/workspace", "readonly"}) {
				t.Fatalf("incorrect mount encoding %q: %v", mount, err)
			}
			var request runrequest.Request
			if err := json.Unmarshal(command.Input, &request); err != nil {
				t.Fatal(err)
			}
			php := processFunc(func(context.Context, runner.Command) runner.Result {
				return runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","php_version":"8.4.26","findings":[{"path":"src/demo.php","line":2,"function":"var_dump"}]}`)}
			})
			result := worker.Execute(ctx, location.Root, request, php)
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			return runner.Result{Stdout: data, ExitCode: result.ExitCode, Err: errors.New("simulated nonzero worker exit")}
		case "rm":
			if ctx.Err() != nil {
				t.Fatal("cleanup must have an independent context")
			}
			return runner.Result{}
		default:
			t.Fatalf("unexpected Docker invocation: %v", command.Args)
			return runner.Result{}
		}
	}
}

func TestDockerTransportUsesPinnedImageAndVersionedRequest(t *testing.T) {
	location, effective, options := fixture(t)
	var calls []runner.Command
	result := Check(context.Background(), location, effective, options, []string{"debug"}, dockerDouble(t, location, &calls))
	if result.ExitCode != 1 || !result.Complete || len(result.Findings) != 1 || *result.Environment.Image != testImage {
		t.Fatalf("wrong worker result: %+v", result)
	}
	if len(calls) != 4 || calls[3].Args[0] != "rm" || calls[3].Args[2] != calls[2].Args[slices.Index(calls[2].Args, "--name")+1] {
		t.Fatalf("container cleanup did not match run: %+v", calls)
	}
}

func TestNativeCheckerTransportAndPolicyValidation(t *testing.T) {
	for _, id := range []string{"whitespace", "drupal-config"} {
		for _, mutation := range []string{"", "wrong kind", "wrong policy", "old capabilities"} {
			t.Run(id+"/"+mutation, func(t *testing.T) {
				location, effective, options := fixture(t)
				filename, data, rule := "src/demo.php", "<?php\n// trailing \n", "whitespace.trailing"
				if id == "drupal-config" {
					filename, data, rule = "src/modules/demo/config/install/demo.yml", "uuid: old\n", "drupal.config.forbidden_key"
				}
				filename = filepath.Join(location.Root, filename)
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
				var calls []runner.Command
				base := dockerDouble(t, location, &calls)
				process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
					result := base(ctx, command)
					if mutation == "old capabilities" && command.Args[0] == "image" {
						result.Stdout = bytesReplace(result.Stdout, "composer-audit,composer-validate,debug,drupal-config,phpcs,phpmd,phpstan,syntax,twig-cs-fixer,whitespace,yaml", "debug,syntax,yaml")
					}
					if command.Args[0] == "run" {
						if mutation == "wrong kind" {
							result.Stdout = bytesReplace(result.Stdout, `"kind":"code"`, `"kind":"syntax"`)
						} else if mutation == "wrong policy" {
							result.Stdout = bytesReplace(result.Stdout, `"required":true`, `"required":false`)
						}
					}
					return result
				})
				result := Check(context.Background(), location, effective, options, []string{id}, process)
				if mutation != "" {
					if result.ExitCode != 3 || result.Complete {
						t.Fatalf("invalid native checker report/image accepted: %+v", result)
					}
				} else if result.ExitCode != 1 || !result.Complete || len(result.Findings) != 1 || result.Findings[0].Rule != rule || result.Environment.PHPRuntime != nil {
					t.Fatalf("native checker transport failed: %+v", result)
				}
			})
		}
	}
}

func TestDockerFailuresCannotReturnClean(t *testing.T) {
	for _, which := range []string{"missing docker", "missing image", "wrong labels", "malformed", "missing report", "wrong exit", "wrong run", "wrong path", "changed file", "new file", "changed config"} {
		t.Run(which, func(t *testing.T) {
			location, effective, options := fixture(t)
			var calls []runner.Command
			base := dockerDouble(t, location, &calls)
			process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
				if which == "missing docker" && command.Args[0] == "context" || which == "missing image" && command.Args[0] == "image" {
					return runner.Result{ExitCode: -1, Err: errors.New("unavailable"), Unavailable: true}
				}
				result := base(ctx, command)
				if which == "wrong labels" && command.Args[0] == "image" {
					result.Stdout = bytesReplace(result.Stdout, `"io.phpwarden.protocol":"2"`, `"io.phpwarden.protocol":"1"`)
				}
				if command.Args[0] != "run" {
					return result
				}
				switch which {
				case "malformed":
					result.Stdout = []byte(`{"version":1}`)
				case "missing report":
					result.Stdout, result.ExitCode = nil, 0
				case "wrong exit":
					result.ExitCode = 0
				case "wrong run":
					var document map[string]any
					json.Unmarshal(result.Stdout, &document)
					document["run_id"] = "other-run"
					result.Stdout, _ = json.Marshal(document)
				case "wrong path":
					result.Stdout = bytesReplace(result.Stdout, `"path":"src/demo.php"`, `"path":"../other.php"`)
				case "changed file", "new file", "changed config":
					filename := filepath.Join(location.Root, "src", "demo.php")
					if which == "new file" {
						filename = filepath.Join(location.Root, "src", "new.php")
					} else if which == "changed config" {
						filename = location.Path
					}
					if err := os.WriteFile(filename, []byte("changed\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				return result
			})
			result := Check(context.Background(), location, effective, options, nil, process)
			if result.ExitCode != 3 || result.Complete || result.Verdict != "error" {
				t.Fatalf("failure accepted: %+v", result)
			}
			if len(calls) > 2 && calls[len(calls)-1].Args[0] != "rm" {
				t.Fatal("container cleanup was not attempted")
			}
		})
	}
}

func bytesReplace(data []byte, old, replacement string) []byte {
	return []byte(strings.ReplaceAll(string(data), old, replacement))
}

func TestCancellationAndTimeoutCleanUpContainers(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		location, effective, options := fixture(t)
		if timeout {
			effective.Config.Runner.TimeoutSeconds = 1
		}
		ctx, cancel := context.WithCancel(context.Background())
		var calls []runner.Command
		base := dockerDouble(t, location, &calls)
		process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
			if command.Args[0] == "run" {
				calls = append(calls, command)
				if !timeout {
					cancel()
				}
				<-ctx.Done()
				return runner.Result{ExitCode: -1, Err: ctx.Err(), TimedOut: timeout}
			}
			return base(ctx, command)
		})
		result := Check(ctx, location, effective, options, nil, process)
		cancel()
		want := 130
		if timeout {
			want = 3
		}
		if result.ExitCode != want || result.Complete || calls[len(calls)-1].Args[0] != "rm" {
			t.Fatalf("incorrect cancellation: %+v calls=%v", result, calls)
		}
	}
}

func TestEmptyScopeNeedsNoDockerAndRemoteContextsAreRejected(t *testing.T) {
	location, effective, options := fixture(t)
	if err := os.Remove(filepath.Join(location.Root, "src", "demo.php")); err != nil {
		t.Fatal(err)
	}
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("empty scope must not invoke a process")
		return runner.Result{}
	})
	result := Check(context.Background(), location, effective, options, nil, unused)
	if result.ExitCode != 0 || !result.Complete || result.Checks[0].Status != "skipped" {
		t.Fatalf("empty scope failed: %+v", result)
	}
	t.Setenv("DOCKER_HOST", "ssh://remote.example")
	if err := localEndpoint(context.Background(), unused); err == nil {
		t.Fatal("remote endpoint accepted")
	}
}
