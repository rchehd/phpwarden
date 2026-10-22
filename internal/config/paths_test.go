package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, root, name string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, name)
	if err := os.WriteFile(filename, []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	return filename
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func TestResolveNearestConfigInMonorepo(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "--quiet", "--initial-branch=main")
	writeConfig(t, root, Filename)
	project := filepath.Join(root, "sites", "demo")
	filename := writeConfig(t, project, Filename)
	cwd := filepath.Join(project, "src", "deep")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", "/a/different/git/directory")
	location, err := Resolve(LocationOptions{CWD: cwd})
	if err != nil {
		t.Fatal(err)
	}
	if location.Root != project || location.Path != filename {
		t.Fatalf("wrong project chosen: %+v", location)
	}
}

func TestResolveDoesNotSearchAboveGitRoot(t *testing.T) {
	parent := t.TempDir()
	writeConfig(t, parent, Filename)
	root := filepath.Join(parent, "repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--quiet", "--initial-branch=main")
	if _, err := Resolve(LocationOptions{CWD: root}); err == nil {
		t.Fatal("must not inherit another project's config above Git root")
	}
}

func TestExplicitLocationsWithoutGit(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project with spaces")
	filename := writeConfig(t, project, "quality.yaml")
	location, err := Resolve(LocationOptions{CWD: root, Config: "project with spaces/quality.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if location.Path != filename || location.Root != project {
		t.Fatalf("wrong explicit location: %+v", location)
	}
	writeConfig(t, project, Filename)
	location, err = Resolve(LocationOptions{CWD: root, Project: "project with spaces"})
	if err != nil || location.Root != project {
		t.Fatalf("explicit project failed: %+v, %v", location, err)
	}
	_, err = Resolve(LocationOptions{CWD: root, Project: root, Config: filename})
	if err == nil {
		t.Fatal("mismatched --project and --config must fail")
	}
}

func TestResolveRealGitWorktree(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	writeConfig(t, root, Filename)
	git(t, root, "init", "--quiet", "--initial-branch=main")
	git(t, root, "add", Filename)
	git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "baseline")
	worktree := filepath.Join(parent, "worktree")
	git(t, root, "worktree", "add", "--quiet", "-b", "fixture", worktree)
	location, err := Resolve(LocationOptions{CWD: worktree})
	if err != nil || location.Root != worktree {
		t.Fatalf("worktree .git file not handled: %+v, %v", location, err)
	}
}

func TestConfigSymlinkCannotChangeProjectRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := writeConfig(t, outside, Filename)
	if err := os.Symlink(target, filepath.Join(root, Filename)); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(LocationOptions{CWD: root, Project: root}); err == nil {
		t.Fatal("symlink silently moved project root")
	}
}

func TestLoadValidatesRootedPaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	for _, testcase := range []struct {
		name    string
		include string
		valid   bool
	}{
		{"root", ".", true},
		{"future source directory", "src/not-created-yet", true},
		{"existing external symlink", "outside", false},
		{"missing child of external symlink", "outside/not-created-yet", false},
		{"broken external symlink", "broken/subdirectory", false},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			filename := writeConfig(t, root, Filename)
			data := minimal + "scope:\n  include: [" + testcase.include + "]\n"
			if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(Location{Root: root, Path: filename})
			if (err == nil) != testcase.valid {
				t.Fatalf("valid=%v, got %v", testcase.valid, err)
			}
		})
	}
}
