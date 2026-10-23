package gitleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func write(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func settings(t *testing.T) config.Gitleaks {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return effective.Config.Tools.Gitleaks
}

func TestFindingsUseChangedLinesAndNeverExposeSecrets(t *testing.T) {
	root := t.TempDir()
	name := "src/key file.txt"
	write(t, root, name, "synthetic fixture\n")
	selected := scope.Selection{Mode: "diff", Files: []string{name}, LineMap: map[string][]scope.Range{name: {{Start: 5, End: 5}}}}
	secret := "synthetic-value-that-must-never-be-reported"
	process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
		if slices.Equal(cmd.Args, []string{"version"}) {
			return runner.Result{Stdout: []byte(Version)}
		}
		for _, required := range []string{"--redact=100", "--log-level=error", "--max-archive-depth=0", "--max-decode-depth=0", "--", name} {
			if !slices.Contains(cmd.Args, required) {
				t.Fatalf("unsafe invocation: missing %s", required)
			}
		}
		if slices.ContainsFunc(cmd.Env, func(value string) bool { return strings.HasPrefix(value, "GITLEAKS_") }) {
			t.Fatal("Gitleaks environment must not change project policy")
		}
		rows := []finding{
			{RuleID: "test-key", File: name, StartLine: 2, EndLine: 2, Secret: "REDACTED", Match: secret},
			{RuleID: "test-key", File: name, StartLine: 4, EndLine: 6, Secret: "REDACTED", Description: secret},
		}
		data, _ := json.Marshal(rows)
		for _, arg := range cmd.Args {
			if file, ok := strings.CutPrefix(arg, "--report-path="); ok {
				if err := os.WriteFile(file, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		return runner.Result{ExitCode: findingsExit, Err: fmt.Errorf("exit status 23")}
	})
	for _, policy := range []string{"error", "warning", "never"} {
		cfg := settings(t)
		cfg.FailOn = policy
		result := run(context.Background(), root, ".", selected, cfg, "gitleaks", process)
		if len(result.Findings) != 1 || *result.Findings[0].Line != 5 || (result.Check.Status == "failed") != (policy != "never") {
			t.Fatalf("wrong findings: %+v", result)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), secret) {
			t.Fatal("native secret or source text leaked into report")
		}
	}
}

func TestGitleaksFailuresCannotLookCleanOrExposeOutput(t *testing.T) {
	for _, which := range []string{"missing", "wrong version", "timeout", "stderr", "crash", "unredacted", "malformed", "foreign file", "exit mismatch"} {
		t.Run(which, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "key.txt", "synthetic fixture")
			secret := "native-value-not-for-diagnostics"
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if slices.Equal(cmd.Args, []string{"version"}) {
					if which == "missing" {
						return runner.Result{Unavailable: true}
					}
					if which == "wrong version" {
						return runner.Result{Stdout: []byte("8.0.0")}
					}
					return runner.Result{Stdout: []byte(Version)}
				}
				if which == "timeout" {
					return runner.Result{TimedOut: true}
				}
				if which == "stderr" || which == "crash" {
					return runner.Result{ExitCode: 1, Stdout: []byte(secret), Stderr: []byte(secret)}
				}
				row := finding{RuleID: "test-key", File: "key.txt", StartLine: 1, EndLine: 1, Secret: "REDACTED"}
				if which == "unredacted" {
					row.Secret = secret
				}
				if which == "foreign file" {
					row.File = "../outside.txt"
				}
				data, _ := json.Marshal([]finding{row})
				if which == "malformed" {
					data = []byte(`{"` + secret + `":invalid}`)
				}
				for _, arg := range cmd.Args {
					if file, ok := strings.CutPrefix(arg, "--report-path="); ok {
						if err := os.WriteFile(file, data, 0o600); err != nil {
							t.Fatal(err)
						}
					}
				}
				if which == "exit mismatch" {
					return runner.Result{}
				}
				return runner.Result{ExitCode: findingsExit}
			})
			selected := scope.Selection{Files: []string{"key.txt"}, LineMap: map[string][]scope.Range{"key.txt": nil}}
			result := run(context.Background(), root, ".", selected, settings(t), "gitleaks", process)
			if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Findings) > 0 {
				t.Fatalf("failure looked clean: %+v", result)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), secret) {
				t.Fatal("native error leaked into diagnostics")
			}
		})
	}
}

func TestNativeInputsAndDiscoveryAreFingerprinted(t *testing.T) {
	root := t.TempDir()
	cfg := settings(t)
	before := ContextHash(root, "platform", cfg)
	for _, name := range []string{"platform/.gitleaks.toml", ".gitleaks.toml", ".gitleaksignore"} {
		write(t, root, name, "changed")
		after := ContextHash(root, "platform", cfg)
		if before == after {
			t.Fatalf("new native input did not invalidate context: %s", name)
		}
		before = after
	}
	cfg.Baseline, cfg.ExtraInputs = "baseline.json", []string{"shared.toml"}
	write(t, root, cfg.Baseline, "[]")
	write(t, root, "shared.toml", "before")
	before = ContextHash(root, ".", cfg)
	write(t, root, "shared.toml", "after")
	if before == ContextHash(root, ".", cfg) {
		t.Fatal("transitive input did not invalidate context")
	}
}

func TestActualGitleaksConfigBaselineAndLineFilter(t *testing.T) {
	binary, err := exec.LookPath("gitleaks")
	if err != nil {
		t.Fatal("install pinned Gitleaks before running checker tests")
	}
	root := t.TempDir()
	write(t, root, ".gitleaks.toml", "[[rules]]\nid = 'phpwarden-test'\ndescription = 'Synthetic fixture'\nregex = '''PHPWARDEN_TEST_[A-Z0-9]{16}'''\n")
	secret := "PHPWARDEN" + "_TEST_" + strings.Repeat("A", 16)
	name := "src/key file.txt"
	write(t, root, name, secret+"\nclean\n"+secret+"\n")
	selected := scope.Selection{Files: []string{name}, LineMap: map[string][]scope.Range{name: {{Start: 3, End: 3}}}}
	cfg := settings(t)
	var baseline []byte
	process := processFunc(func(ctx context.Context, cmd runner.Command) runner.Result {
		result := (runner.Exec{}).Run(ctx, cmd)
		for _, arg := range cmd.Args {
			if file, ok := strings.CutPrefix(arg, "--report-path="); ok {
				baseline, _ = os.ReadFile(file)
			}
		}
		return result
	})
	result := run(context.Background(), root, ".", selected, cfg, binary, process)
	if result.Check.Status != "failed" || len(result.Findings) != 1 || *result.Findings[0].Line != 3 {
		t.Fatalf("actual Gitleaks: %+v", result)
	}
	if strings.Contains(string(baseline), secret) {
		t.Fatal("native report did not redact secrets")
	}
	write(t, root, "baseline.json", string(baseline))
	cfg.Baseline = "baseline.json"
	result = run(context.Background(), root, ".", selected, cfg, binary, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("native baseline not applied: %+v", result)
	}
}
