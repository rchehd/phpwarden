package githook

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInstalledHookExecutesQuotedLauncherAndPropagatesFailure(t *testing.T) {
	directory := t.TempDir()
	runGit(t, directory, "init", "--quiet")
	executable := filepath.Join(t.TempDir(), "phpwarden ' $(touch injected)")
	record := filepath.Join(t.TempDir(), "arguments")
	t.Setenv("PHPWARDEN_HOOK_PROBE", record)
	probe := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PHPWARDEN_HOOK_PROBE\"\nexit 7\n"
	if err := os.WriteFile(executable, []byte(probe), 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareInstall(context.Background(), directory, executable)
	if err != nil {
		t.Fatal(err)
	}
	installed, err := plan.Install()
	if err != nil || !installed.Created {
		t.Fatalf("install: %+v %v", installed, err)
	}
	command := exec.Command("git", "-C", directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "blocked")
	if data, err := command.CombinedOutput(); err == nil {
		t.Fatalf("failing launcher allowed commit: %s", data)
	}
	if data, _ := os.ReadFile(record); string(data) != "hook\npre-commit\n" {
		t.Fatalf("launcher was not invoked correctly: %q", data)
	}
	if _, err := os.Stat(filepath.Join(directory, "injected")); !os.IsNotExist(err) {
		t.Fatal("launcher filename was interpreted as shell code")
	}
	if err := os.WriteFile(executable, []byte(strings.Replace(probe, "exit 7", "exit 0", 1)), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "allowed")
}

func TestConcurrentInstallsCreateExactlyOneHook(t *testing.T) {
	directory := t.TempDir()
	runGit(t, directory, "init", "--quiet")
	plan, err := PrepareInstall(context.Background(), directory, "phpwarden")
	if err != nil {
		t.Fatal(err)
	}
	results := make([]Installation, 8)
	errors := make([]error, 8)
	var group sync.WaitGroup
	for index := range results {
		group.Go(func() { results[index], errors[index] = plan.Install() })
	}
	group.Wait()
	created := 0
	for index, result := range results {
		if errors[index] != nil {
			t.Fatal(errors[index])
		}
		if result.Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created %d hooks", created)
	}
	if entries, _ := os.ReadDir(filepath.Dir(results[0].Path)); len(entries) == 0 {
		t.Fatal("installed hook is missing")
	} else {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".phpwarden-hook-") {
				t.Fatal("temporary installer file was not cleaned up")
			}
		}
	}
}

func TestInstallerPreservesSymlinksAndExternalHookDirectories(t *testing.T) {
	for _, kind := range []string{"file symlink", "directory symlink", "external hooksPath"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			runGit(t, directory, "init", "--quiet")
			outside := t.TempDir()
			original := []byte("#!/bin/sh\nexit 23\n")
			if err := os.WriteFile(filepath.Join(outside, "pre-commit"), original, 0o755); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "file symlink":
				if err := os.Symlink(filepath.Join(outside, "pre-commit"), filepath.Join(directory, ".git", "hooks", "pre-commit")); err != nil {
					t.Fatal(err)
				}
			case "directory symlink":
				if err := os.Symlink(outside, filepath.Join(directory, ".githooks")); err != nil {
					t.Fatal(err)
				}
				runGit(t, directory, "config", "core.hooksPath", ".githooks")
			case "external hooksPath":
				runGit(t, directory, "config", "core.hooksPath", outside)
			}
			if _, err := PrepareInstall(context.Background(), directory, "phpwarden"); err == nil {
				t.Fatal("unsafe or shared hook destination accepted")
			}
			if data, _ := os.ReadFile(filepath.Join(outside, "pre-commit")); !bytes.Equal(data, original) {
				t.Fatal("external hook was modified")
			}
		})
	}
}

func TestLinkedWorktreeUsesCommonGitHookDirectory(t *testing.T) {
	directory := t.TempDir()
	runGit(t, directory, "init", "--quiet")
	runGit(t, directory, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "baseline")
	worktree := filepath.Join(t.TempDir(), "linked worktree")
	runGit(t, directory, "worktree", "add", "--detach", worktree)
	plan, err := PrepareInstall(context.Background(), worktree, "phpwarden")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := plan.Install()
	if err != nil || installed.Path != filepath.Join(directory, ".git", "hooks", "pre-commit") {
		t.Fatalf("linked-worktree hook: %+v %v", installed, err)
	}
}
