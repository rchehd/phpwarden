package perfruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
)

type fakeProcess struct {
	calls  []runner.Command
	docker func(runner.Command) runner.Result
}

func (p *fakeProcess) Run(ctx context.Context, c runner.Command) runner.Result {
	p.calls = append(p.calls, c)
	if c.Name == "git" {
		return (runner.Exec{}).Run(ctx, c)
	}
	if p.docker != nil {
		return p.docker(c)
	}
	return runner.Result{ExitCode: 1, Stderr: []byte("unexpected runtime command")}
}

func success(data any) runner.Result {
	encoded, _ := json.Marshal(data)
	return runner.Result{Stdout: encoded}
}

func write(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func git(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	r := (runner.Exec{}).Run(context.Background(), runner.Command{Name: "git", Args: args, Directory: dir})
	if r.Err != nil || r.ExitCode != 0 {
		t.Fatalf("git %v: %s", args, r.Stderr)
	}
	return r.Stdout
}

func ddevFixture(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if err := os.MkdirAll(filepath.Join(dir, ".ddev/web-build"), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, ".ddev/config.yaml"), []byte("name: fixture\nphp_version: '8.3'\n"))
	write(t, filepath.Join(dir, "untracked.txt"), []byte("user contents"))
	git(t, dir, "add", ".ddev/config.yaml")
	return Options{Project: dir, StateDirectory: filepath.Join(t.TempDir(), "private-state"), Process: &fakeProcess{}, readArchive: func(context.Context, string) ([]byte, error) { return []byte("verified-source-boundary"), nil }}
}

func prepared(t *testing.T, o Options) (workspace, Report) {
	t.Helper()
	r := Run(context.Background(), "setup", o)
	if r.ExitCode != 0 || !r.Prepared || r.Loaded || r.State != "prepared" {
		t.Fatalf("%+v", r)
	}
	w, err := locate(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.load(); err != nil {
		t.Fatal(err)
	}
	return w, r
}

func TestDDEVPreparationIdempotenceAndOwnedTeardown(t *testing.T) {
	o := ddevFixture(t)
	o.HTTP = true
	before := git(t, o.Project, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	exclude := filepath.Join(o.Project, ".git/info/exclude")
	original := []byte("# user exclusion\n*.local\n")
	write(t, exclude, original)
	if err := os.Chmod(exclude, 0640); err != nil {
		t.Fatal(err)
	}
	w, r := prepared(t, o)
	if !bytes.Equal(before, git(t, o.Project, "status", "--porcelain=v1", "-z", "--untracked-files=all")) {
		t.Fatal("Git status changed")
	}
	stateBefore, err := os.ReadFile(filepath.Join(w.directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r2 := Run(context.Background(), "setup", o); r2.ExitCode != 0 || r2.Loaded {
		t.Fatalf("%+v", r2)
	}
	stateAfter, _ := os.ReadFile(filepath.Join(w.directory, "state.json"))
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("repeated setup rewrote state")
	}
	key, _ := os.ReadFile(r.HTTPKeyFile)
	key = bytes.TrimSpace(key)
	encoded, _ := json.Marshal(r)
	for _, data := range [][]byte{encoded, stateAfter, w.dockerfile(), w.overlay()} {
		if bytes.Contains(data, key) {
			t.Fatal("HTTP key leaked out of private configuration")
		}
	}
	ignore, _ := os.ReadFile(filepath.Join(w.directory, ".dockerignore"))
	if !bytes.Contains(ignore, []byte("**\n")) || bytes.Contains(ignore, []byte("!http.key")) {
		t.Fatal("private reports or key enter Docker context")
	}
	write(t, filepath.Join(r.Reports, "preserved.json"), []byte("user recording"))
	if result := Run(context.Background(), "teardown", o); result.ExitCode != 0 || result.State != "removed" {
		t.Fatalf("%+v", result)
	}
	if _, err := os.Stat(filepath.Join(r.Reports, "preserved.json")); err != nil {
		t.Fatal("teardown deleted profiles")
	}
	got, _ := os.ReadFile(exclude)
	info, _ := os.Stat(exclude)
	if !bytes.Equal(got, original) || info.Mode().Perm() != 0640 {
		t.Fatal("Git excludes were not restored exactly")
	}
	if result := Run(context.Background(), "teardown", o); result.ExitCode != 0 {
		t.Fatalf("%+v", result)
	}
	prepared(t, o)
	for _, call := range o.Process.(*fakeProcess).calls {
		if call.Name != "git" {
			t.Fatalf("preparation executed %s", call.Name)
		}
	}
}

func TestChangedOwnedFileAndSymlinkAreNeverRemoved(t *testing.T) {
	for _, kind := range []string{"changed", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			o := ddevFixture(t)
			w, _ := prepared(t, o)
			target := filepath.Join(o.Project, projectNames[0])
			if kind == "changed" {
				write(t, target, []byte("user edit"))
			} else {
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(t.TempDir(), "user.txt")
				write(t, outside, []byte("keep"))
				if err := os.Symlink(outside, target); err != nil {
					t.Fatal(err)
				}
			}
			for _, action := range []string{"setup", "status", "teardown"} {
				if r := Run(context.Background(), action, o); r.ExitCode != 3 {
					t.Fatalf("%s: %+v", action, r)
				}
			}
			if _, err := os.Lstat(target); err != nil {
				t.Fatal("user artifact was removed")
			}
			if _, err := os.Stat(filepath.Join(w.directory, "state.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChangedRuntimeInputsStillAllowRemovingLocalAdapters(t *testing.T) {
	o := ddevFixture(t)
	prepared(t, o)
	write(t, filepath.Join(o.Project, ".ddev/config.yaml"), []byte("name: fixture\nphp_version: '8.4'\n"))
	if r := Run(context.Background(), "status", o); r.ExitCode != 3 {
		t.Fatalf("%+v", r)
	}
	if r := Run(context.Background(), "teardown", o); r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
	data, _ := os.ReadFile(filepath.Join(o.Project, ".ddev/config.yaml"))
	if !bytes.Contains(data, []byte("8.4")) {
		t.Fatal("user config changed")
	}
}

func TestReservedTrackedAndExistingPathsAreRefused(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(fmt.Sprint(tracked), func(t *testing.T) {
			o := ddevFixture(t)
			name := filepath.Join(o.Project, projectNames[0])
			write(t, name, []byte("existing"))
			if tracked {
				git(t, o.Project, "add", projectNames[0])
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
			}
			if r := Run(context.Background(), "setup", o); r.ExitCode == 0 {
				t.Fatalf("%+v", r)
			}
			if !tracked {
				data, _ := os.ReadFile(name)
				if string(data) != "existing" {
					t.Fatal("overwrote an existing file")
				}
			}
		})
	}
}

func TestLinkedWorktreeAndNestedProjectUseCheckoutIdentity(t *testing.T) {
	o := ddevFixture(t)
	git(t, o.Project, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture")
	second := filepath.Join(t.TempDir(), "other checkout")
	git(t, o.Project, "worktree", "add", "--detach", second)
	first, _ := prepared(t, o)
	other := o
	other.Project = second
	secondState, _ := prepared(t, other)
	if first.directory == secondState.directory {
		t.Fatal("worktrees share runtime state")
	}
	if r := Run(context.Background(), "teardown", o); r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(second, projectNames[0])); err != nil {
		t.Fatal("teardown touched another worktree")
	}
	if r := Run(context.Background(), "teardown", other); r.ExitCode != 0 {
		t.Fatalf("%+v", r)
	}
	nested := filepath.Join(o.Project, "nested project")
	if err := os.MkdirAll(filepath.Join(nested, ".ddev/web-build"), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(nested, ".ddev/config.yaml"), []byte("name: nested\nphp_version: '8.3'\n"))
	other.Project = nested
	before := git(t, nested, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	prepared(t, other)
	if !bytes.Equal(before, git(t, nested, "status", "--porcelain=v1", "-z", "--untracked-files=all")) {
		t.Fatal("nested project adapters are visible in Git")
	}
}

func TestUnsafeStateAndSourceArchiveAreRefused(t *testing.T) {
	o := ddevFixture(t)
	o.StateDirectory = filepath.Join(o.Project, "local-state")
	if r := Run(context.Background(), "setup", o); r.ExitCode != 2 {
		t.Fatalf("%+v", r)
	}
	o = ddevFixture(t)
	bad := filepath.Join(t.TempDir(), "spx.tar.gz")
	write(t, bad, []byte("wrong archive"))
	o.SourceArchive = bad
	o.readArchive = nil
	if r := Run(context.Background(), "setup", o); r.ExitCode != 3 {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(o.Project, projectNames[0])); !os.IsNotExist(err) {
		t.Fatal("unverified source created an adapter")
	}
	o = ddevFixture(t)
	w, r := prepared(t, o)
	if _, err := os.Stat(r.HTTPKeyFile); err == nil {
		t.Fatal("unexpected HTTP key")
	}
	write(t, filepath.Join(w.directory, "state.json"), []byte(`{"version":1,"project":"wrong"}`))
	if result := Run(context.Background(), "teardown", o); result.ExitCode != 3 {
		t.Fatalf("%+v", result)
	}
}

func composeFixture(t *testing.T) (Options, *fakeProcess) {
	t.Helper()
	o := ddevFixture(t)
	o.Environment = "compose"
	o.PHP = "8.2"
	o.ComposeFiles = []string{"compose.yaml", "compose.dev.yaml"}
	o.EnvFiles = []string{"dev.env"}
	o.Profiles = []string{"dev"}
	o.Service = "php"
	o.ComposeProject = "fixture"
	o.Image = "php:local"
	o.ContainerRoot = "/project"
	for _, name := range append(append([]string{}, o.ComposeFiles...), o.EnvFiles...) {
		write(t, filepath.Join(o.Project, name), []byte("fixture input"))
	}
	p := &fakeProcess{}
	p.docker = func(c runner.Command) runner.Result {
		switch {
		case slices.Equal(c.Args, []string{"context", "inspect", "--format", "{{json .Endpoints.docker.Host}}"}):
			return success("unix:///var/run/docker.sock")
		case slices.Contains(c.Args, "config"):
			return success(map[string]any{"name": "fixture", "services": map[string]any{"php": map[string]any{"image": "php:local", "environment": map[string]string{"SECRET": "private-environment-value"}}}})
		case slices.Equal(c.Args, []string{"image", "inspect", "php:local"}):
			return success([]any{map[string]any{"Id": "sha256:" + strings.Repeat("a", 64), "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"User": "33:33"}}})
		case slices.Contains(c.Args, "ps"):
			return runner.Result{}
		default:
			return runner.Result{ExitCode: 1, Stderr: []byte("unexpected command")}
		}
	}
	o.Process = p
	return o, p
}

func TestComposePreservesInvocationAndNeverPersistsEnvironment(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	t.Setenv("COMPOSE_FILE", "")
	t.Setenv("COMPOSE_PROFILES", "")
	o, p := composeFixture(t)
	w, r := prepared(t, o)
	data, _ := os.ReadFile(filepath.Join(w.directory, "state.json"))
	encoded, _ := json.Marshal(r)
	if bytes.Contains(data, []byte("private-environment-value")) || bytes.Contains(encoded, []byte("private-environment-value")) {
		t.Fatal("persisted raw environment")
	}
	args := w.composeArgs(true)
	for _, value := range []string{"--project-directory", o.Project, "-p", "fixture", "--profile", "dev", "--env-file", filepath.Join(o.Project, "dev.env"), filepath.Join(o.Project, "compose.yaml"), filepath.Join(o.Project, "compose.dev.yaml"), filepath.Join(w.directory, "compose.yaml")} {
		if !slices.Contains(args, value) {
			t.Fatalf("lost invocation option %q: %v", value, args)
		}
	}
	if r := Run(context.Background(), "status", o); r.ExitCode != 0 || r.Loaded || r.State != "prepared" {
		t.Fatalf("%+v", r)
	}
	for _, call := range p.calls {
		if slices.Contains(call.Args, "build") || slices.Contains(call.Args, "up") {
			t.Fatal("preparation/status mutated runtime")
		}
	}
	if strings.Contains(string(w.overlay()), "private-environment-value") {
		t.Fatal("overlay exposes base env")
	}
}

func TestRemoteDockerAndAmbiguousSelectionAreRefused(t *testing.T) {
	o, _ := composeFixture(t)
	t.Setenv("DOCKER_HOST", "ssh://remote")
	t.Setenv("DOCKER_CONTEXT", "")
	if r := Run(context.Background(), "setup", o); r.ExitCode != 3 {
		t.Fatalf("%+v", r)
	}
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	o.Service = ""
	if r := Run(context.Background(), "setup", o); r.ExitCode != 3 {
		t.Fatalf("%+v", r)
	}
}

func TestCancellationAndAbsentStatus(t *testing.T) {
	o := ddevFixture(t)
	before, err := locate(o)
	if err != nil {
		t.Fatal(err)
	}
	if r := Run(context.Background(), "status", o); r.ExitCode != 0 || r.State != "absent" {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(before.directory); !os.IsNotExist(err) {
		t.Fatal("status created runtime state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := Run(ctx, "setup", o); r.ExitCode != 130 {
		t.Fatalf("%+v", r)
	}
}

func TestComposeApplyIsScopedAndRestoreCanResume(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	t.Setenv("COMPOSE_FILE", "")
	t.Setenv("COMPOSE_PROFILES", "")
	o, p := composeFixture(t)
	w, _ := prepared(t, o)
	fallback := p.docker
	active, failRestore := false, false
	derived := "sha256:" + strings.Repeat("b", 64)
	p.docker = func(c runner.Command) runner.Result {
		switch {
		case slices.Contains(c.Args, "ps"):
			if active {
				return runner.Result{Stdout: []byte("fixture-id\n")}
			}
			return runner.Result{}
		case len(c.Args) > 1 && c.Args[0] == "container" && c.Args[1] == "inspect":
			return success([]any{map[string]any{"Id": "fixture-id", "Image": derived, "State": map[string]bool{"Running": true}, "Config": map[string]any{"Labels": map[string]string{ownerLabel: w.owner, producerLabel: source().Commit}}, "Mounts": []any{map[string]any{"Source": filepath.Join(w.directory, "reports"), "Destination": containerReports, "RW": true}, map[string]any{"Source": filepath.Join(w.directory, "spx.ini"), "Destination": "/usr/local/etc/php/conf.d/99-phpwarden-spx.ini", "RW": false}}}})
		case len(c.Args) > 0 && c.Args[0] == "exec":
			return success(map[string]any{"loaded": true, "php": "8.2", "producer": source().Commit, "reports": containerReports, "writable": true, "config_hash": w.state.Files["spx.ini"]})
		case slices.Contains(c.Args, "build") || len(c.Args) > 1 && c.Args[0] == "image" && c.Args[1] == "tag":
			return runner.Result{}
		case len(c.Args) > 2 && c.Args[0] == "image" && c.Args[1] == "inspect" && c.Args[2] == "--format":
			return runner.Result{Stdout: []byte(derived + "\n")}
		case slices.Contains(c.Args, "up"):
			if c.Args[len(c.Args)-1] != "php" || !slices.Contains(c.Args, "--no-deps") || !slices.Contains(c.Args, "--no-build") {
				t.Fatal("runtime action can affect sibling services")
			}
			if slices.Contains(c.Args, filepath.Join(w.directory, "compose.yaml")) {
				active = true
			} else {
				if failRestore {
					return runner.Result{ExitCode: 1, Stderr: []byte("private failure details")}
				}
				active = false
			}
			return runner.Result{}
		default:
			return fallback(c)
		}
	}
	o.Apply = true
	r := Run(context.Background(), "setup", o)
	if r.ExitCode != 0 || !r.Loaded || !r.RuntimeChecked || r.State != "active" {
		t.Fatalf("%+v", r)
	}
	count := len(p.calls)
	r = Run(context.Background(), "setup", o)
	if r.ExitCode != 0 || !r.Loaded {
		t.Fatalf("%+v", r)
	}
	for _, call := range p.calls[count:] {
		if slices.Contains(call.Args, "build") || slices.Contains(call.Args, "up") {
			t.Fatal("repeated apply rebuilt/restarted an already verified runtime")
		}
	}
	failRestore = true
	r = Run(context.Background(), "teardown", o)
	if r.ExitCode != 3 || strings.Contains(strings.Join(r.Diagnostics, " "), "private failure") {
		t.Fatalf("%+v", r)
	}
	if err := w.load(); err != nil || w.state.Phase != "removing" {
		t.Fatalf("%v %+v", err, w.state)
	}
	failRestore = false
	r = Run(context.Background(), "teardown", o)
	if r.ExitCode != 0 || r.State != "removed" || active {
		t.Fatalf("%+v", r)
	}
	count = len(p.calls)
	r = Run(context.Background(), "teardown", o)
	if r.ExitCode != 0 || len(p.calls) != count {
		t.Fatal("repeated applied teardown restarted the runtime")
	}
}

func TestBareDockerPreparationNeverClonesContainer(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")
	t.Setenv("DOCKER_CONTEXT", "")
	o, p := composeFixture(t)
	o.Environment = "docker"
	o.Service = ""
	o.Container = "fixture-php"
	o.ComposeFiles = nil
	o.EnvFiles = nil
	o.Profiles = nil
	o.ComposeProject = ""
	w, r := prepared(t, o)
	if len(r.LaunchArguments) == 0 || r.Loaded {
		t.Fatalf("%+v", r)
	}
	data, _ := os.ReadFile(filepath.Join(w.directory, "spx.ini"))
	if bytes.Contains(data, []byte("http_enabled=1")) {
		t.Fatal("HTTP profiling enabled without opt-in")
	}
	info, _ := os.Stat(filepath.Join(w.directory, "spx.ini"))
	if info.Mode().Perm() != 0644 {
		t.Fatal("container PHP user cannot read its mounted configuration")
	}
	for _, call := range p.calls {
		for _, forbidden := range []string{"stop", "rm", "run", "create", "rename"} {
			if slices.Contains(call.Args, forbidden) {
				t.Fatal("cloned/mutated a bare Docker container")
			}
		}
	}
	o.Apply = true
	if result := Run(context.Background(), "teardown", o); result.ExitCode != 3 {
		t.Fatalf("%+v", result)
	}
}

func TestHTTPConfigurationAndSavedFlagInjectionAreRejected(t *testing.T) {
	o := ddevFixture(t)
	o.HTTP = true
	o.HTTPAllow = "127.0.0.1\nspx.http_key=bad"
	if r := Run(context.Background(), "setup", o); r.ExitCode == 0 {
		t.Fatalf("%+v", r)
	}
	o = ddevFixture(t)
	w, _ := prepared(t, o)
	w.state.Binding.Service = "--privileged"
	if err := w.save(); err != nil {
		t.Fatal(err)
	}
	if r := Run(context.Background(), "status", o); r.ExitCode != 3 {
		t.Fatalf("%+v", r)
	}
}
