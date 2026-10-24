package githook

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func runGit(t *testing.T, directory string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, data)
	}
	return data
}

func TestReadsAlternateIndexWithoutChangingTheNormalIndex(t *testing.T) {
	directory := t.TempDir()
	runGit(t, directory, "init", "--quiet")
	filename := "staged file\nwith spaces.php"
	if err := os.WriteFile(filepath.Join(directory, filename), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(directory, ".git", "temporary-index")
	t.Setenv("GIT_INDEX_FILE", index)
	runGit(t, directory, "add", "--", filename)
	snapshot, err := Read(context.Background(), directory)
	if err != nil || !slices.Equal(snapshot.Files, []string{filename}) {
		t.Fatalf("alternate index was not selected: %+v %v", snapshot, err)
	}
	if err := snapshot.Check(context.Background(), snapshot.Files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, ".git", "index")); !os.IsNotExist(err) {
		t.Fatal("checking an alternate index changed the normal index")
	}
	if err := os.WriteFile(filepath.Join(directory, filename), []byte("<?php\nchanged();\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Check(context.Background(), snapshot.Files); err == nil || !strings.Contains(err.Error(), "unstaged changes") {
		t.Fatalf("alternate index/worktree mismatch was missed: %v", err)
	}
}

func TestIndexChangesInvalidateTheCheck(t *testing.T) {
	directory := t.TempDir()
	runGit(t, directory, "init", "--quiet")
	filename := filepath.Join(directory, "demo.php")
	if err := os.WriteFile(filename, []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, directory, "add", "demo.php")
	snapshot, err := Read(context.Background(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("<?php\nchanged();\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, directory, "add", "demo.php")
	if err := snapshot.Check(context.Background(), snapshot.Files); err == nil || !strings.Contains(err.Error(), "index changed") {
		t.Fatalf("concurrent staging was accepted: %v", err)
	}
}
