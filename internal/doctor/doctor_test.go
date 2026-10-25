package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
)

type fakeProcess struct{ calls []runner.Command }

func (process *fakeProcess) Run(_ context.Context, command runner.Command) runner.Result {
	process.calls = append(process.calls, command)
	if command.Name == "git" {
		return runner.Result{Stdout: []byte(command.Directory)}
	}
	return runner.Result{ExitCode: -1, Unavailable: true}
}

func TestDoctorReportsSetupErrorsWithoutRunningChecksOrChangingFiles(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".phpwarden.yaml": "version: 1\nprofile: drupal\nphp:\n  runtime: '8.3'\n  target: {min: '8.3', max: '8.3'}\ntools:\n  deptrac: {config: deptrac.yaml}\n  rector: {config: rector.php}\n",
		"composer.json":   `{"require":{"drupal/core":"^11"}}`,
		"composer.lock":   `{"packages":[{"name":"drupal/core","version":"11.0.0","require":{"php":">=8.3"}}]}`,
		"deptrac.yaml":    "deptrac: [\n",
		"rector.php":      "<?php throw new Exception('must never execute');",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	process := &fakeProcess{}
	result := Run(context.Background(), config.LocationOptions{CWD: root, Project: root}, process)
	if result.ExitCode != 3 {
		t.Fatalf("%+v", result)
	}
	items := map[string]Item{}
	for _, item := range result.Items {
		items[item.ID] = item
	}
	if items["configuration"].Status != "ok" || items["deptrac_config"].Status != "error" || items["rector_config"].Status != "info" || items["worker_image"].Status != "error" {
		t.Fatalf("%+v", items)
	}
	for _, command := range process.calls {
		if command.Name != "git" && command.Name != "docker" {
			t.Fatalf("doctor executed analyzer %+v", command)
		}
	}
	for name, want := range files {
		data, _ := os.ReadFile(filepath.Join(root, name))
		if string(data) != want {
			t.Fatalf("changed %s", name)
		}
	}
}

func TestDoctorInvalidConfigurationIsStructured(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".phpwarden.yaml"), []byte("version: 1\nprofile: missing\n"), 0o644)
	result := Run(context.Background(), config.LocationOptions{CWD: root, Project: root}, &fakeProcess{})
	if result.ExitCode != 3 || len(result.Items) < 1 || !strings.Contains(result.Items[len(result.Items)-1].Hint, "config validate") {
		t.Fatalf("%+v", result)
	}
}
