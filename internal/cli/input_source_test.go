package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/schemas"
)

func TestLocatedInputDiagnosticsDoNotStartChecks(t *testing.T) {
	t.Setenv("PATH", "")
	for _, item := range []struct{ file, data, position, path string }{
		{".phpwarden.yaml", "version: 1\nprofile: drupal\nprofile: drupal\n", ":3:1", "$.profile"},
		{"composer.json", "{\n  \"require\": }", ":2:14", "$.require"},
	} {
		t.Run(item.file, func(t *testing.T) {
			root := project(t)
			if err := os.WriteFile(filepath.Join(root, item.file), []byte(item.data), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, format := range []string{"json", "text"} {
				var stdout, stderr bytes.Buffer
				code := Run([]string{"check", "--project", root, "--format=" + format}, root, &stdout, &stderr)
				if code != 2 || stderr.Len() != 0 {
					t.Fatalf("exit %d: %s %s", code, &stdout, &stderr)
				}
				if format == "json" {
					var result report.Report
					if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if result.Complete || len(result.Checks) != 0 || len(result.Findings) != 0 || len(result.Diagnostics) != 1 {
						t.Fatalf("unexpected report: %+v", result)
					}
					var document any
					if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
						t.Fatal(err)
					}
					if err := schemas.ValidateReport(document); err != nil {
						t.Fatal(err)
					}
				}
				for _, want := range []string{item.file + item.position, item.path} {
					if !strings.Contains(stdout.String(), want) {
						t.Errorf("missing %q: %s", want, &stdout)
					}
				}
				if format == "text" && (!strings.Contains(stdout.String(), "NOT RUN") || !strings.Contains(stdout.String(), "Fix:")) {
					t.Fatal(stdout.String())
				}
			}
		})
	}
}
