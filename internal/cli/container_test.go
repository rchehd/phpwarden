package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
)

func TestContainerInitUsesHostWrapperAndPreservesHooks(t *testing.T) {
	root := project(t)
	if data, err := exec.Command("git", "-C", root, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, data)
	}
	wrapper := filepath.Join(root, "phpwarden")
	t.Setenv("PHPWARDEN_EXECUTABLE", wrapper)
	for range 2 {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"container", "init", "-n", "--pre-commit", "--format=json"}, root, &stdout, &stderr); code != 0 {
			t.Fatalf("%d %s %s", code, &stdout, &stderr)
		}
		data, _ := os.ReadFile(filepath.Join(root, ".git/hooks/pre-commit"))
		if !strings.Contains(string(data), "\"$project_root\"/'phpwarden'") || strings.Contains(string(data), "/usr/local/bin") {
			t.Fatalf("%s", data)
		}
	}
}

func TestContainerRejectsUnsupportedHostCommands(t *testing.T) {
	for _, args := range [][]string{{"container", "update"}, {"container", "hook", "stop"}, {"container", "init", "--hooks=all"}, {"container", "init", "--pre-commit"}} {
		var stdout, stderr bytes.Buffer
		t.Setenv("PHPWARDEN_EXECUTABLE", "")
		if code := Run(args, t.TempDir(), &stdout, &stderr); code != 2 {
			t.Fatalf("%v: %d %s %s", args, code, &stdout, &stderr)
		}
	}
}

func TestContainerAuditExportContainsOnlyComposerMetadata(t *testing.T) {
	for _, only := range []string{"debug", "composer-audit"} {
		t.Run(only, func(t *testing.T) {
			root, exchange := project(t), t.TempDir()
			for name, data := range map[string]string{"composer.json": "{}", "composer.lock": "{\"packages\":[]}", ".env": "DO_NOT_EXPORT=secret"} {
				os.WriteFile(filepath.Join(root, name), []byte(data), 0o644)
			}
			t.Setenv("PHPWARDEN_AUDIT_EXPORT", exchange)
			var stdout, stderr bytes.Buffer
			if code := Run([]string{"container", "audit-export", "check", "--project", root, "--only", only}, root, &stdout, &stderr); code != 0 || stdout.Len() != 0 {
				t.Fatalf("%d %s %s", code, &stdout, &stderr)
			}
			files, _ := os.ReadDir(exchange)
			want := 1
			if only == "composer-audit" {
				want = 3
			}
			if len(files) != want {
				t.Fatalf("unexpected files %v", files)
			}
			data, _ := os.ReadFile(filepath.Join(exchange, "audit.json"))
			var settings config.ComposerAudit
			if json.Unmarshal(data, &settings) != nil || settings.Enabled != (only == "composer-audit") {
				t.Fatalf("wrong audit settings %s", data)
			}
		})
	}
}

func TestManagedPackageUpdateKeepsJSONAndRefusesNetwork(t *testing.T) {
	t.Setenv("PHPWARDEN_INSTALLATION", "npm")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"update", "--format=json"}, t.TempDir(), &stdout, &stderr)
	if code != 3 || !json.Valid(stdout.Bytes()) || !strings.Contains(stdout.String(), "npm installation") || stderr.Len() != 0 {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
}
