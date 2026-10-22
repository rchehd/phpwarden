package scope

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, root, filename, content string) {
	t.Helper()
	full := filepath.Join(root, filename)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseline(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "--quiet", "--initial-branch=main")
	write(t, root, "src/demo.php", "<?php\n\nvar_dump('legacy');\n$safe = 1;\n")
	write(t, root, ".gitignore", "ignored/\n")
	git(t, root, "add", "--all")
	git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "baseline")
	git(t, root, "switch", "--quiet", "-c", "scenario")
	return root
}

func options(root, mode string) Options {
	return Options{Root: root, CWD: root, Mode: mode, Include: []string{"src"},
		Exclude: []string{"**/vendor/**", "**/node_modules/**"}}
}

func build(t *testing.T, options Options) Selection {
	t.Helper()
	selection, err := Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return selection
}

func TestDiffModesAndNetWorkingTree(t *testing.T) {
	root := baseline(t)
	write(t, root, "src/demo.php", "<?php\n\nvar_dump('legacy');\n$safe = 2;\n")
	git(t, root, "add", "src/demo.php")
	git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "change")
	write(t, root, "src/demo.php", "<?php\n\nvar_dump('legacy');\n$safe = 3;\n")
	git(t, root, "add", "src/demo.php")
	write(t, root, "src/demo.php", "<?php\n\nvar_dump('legacy');\n$safe = 4;\n")
	opts := options(root, "diff")
	opts.Base = "main"
	selection := build(t, opts)
	if !reflect.DeepEqual(selection.Files, []string{"src/demo.php"}) || selection.Contains("src/demo.php", 3) || !selection.Contains("src/demo.php", 4) {
		t.Fatalf("incorrect changed lines: %+v", selection)
	}
	opts.WholeFiles = true
	if !build(t, opts).Contains("src/demo.php", 3) {
		t.Fatal("whole-files must include old debug")
	}
	if !build(t, options(root, "full")).Contains("src/demo.php", 3) {
		t.Fatal("full must include old debug")
	}
	write(t, root, "src/demo.php", "<?php\n\nvar_dump('legacy');\n$safe = 1;\n")
	opts.WholeFiles = false
	if got := build(t, opts); len(got.Files) != 0 {
		t.Fatalf("net change reverted to baseline must be empty: %+v", got)
	}
}

func TestNewFilesRenamesAndDeletion(t *testing.T) {
	root := baseline(t)
	git(t, root, "mv", "src/demo.php", "src/renamed.php")
	write(t, root, "src/new file\nwith tab\t.php", "<?php\nvar_dump('new');\n")
	write(t, root, "src/:(glob)*.php", "<?php\n")
	opts := options(root, "diff")
	opts.Base = "main"
	selection := build(t, opts)
	if len(selection.Files) != 3 || !selection.Contains("src/renamed.php", 3) || !selection.Contains("src/new file\nwith tab\t.php", 2) {
		t.Fatalf("new paths lost or partial: %+v", selection)
	}
	if _, exists := selection.LineMap["src/demo.php"]; exists {
		t.Fatal("deleted path must not remain selected")
	}
}

func TestDeletionOnlyAndBinary(t *testing.T) {
	root := baseline(t)
	write(t, root, "src/demo.php", "<?php\n\n$safe = 1;\n")
	opts := options(root, "diff")
	opts.Base = "main"
	selection := build(t, opts)
	if len(selection.Files) != 1 || selection.LineMap["src/demo.php"] == nil || len(selection.LineMap["src/demo.php"]) != 0 {
		t.Fatalf("deletion-only change needs empty changed-line map: %+v", selection)
	}
	write(t, root, "src/demo.php", "\x00binary\n")
	if build(t, opts).LineMap["src/demo.php"] != nil {
		t.Fatal("changed binary must conservatively select all lines")
	}
}

func TestFullExclusionsAndIgnoredFiles(t *testing.T) {
	root := baseline(t)
	write(t, root, "src/vendor/package/lib.php", "<?php\n")
	write(t, root, "src/generated/a.php", "<?php\n")
	write(t, root, "ignored/file.php", "<?php\n")
	opts := options(root, "full")
	opts.Include = []string{"."}
	opts.Exclude = append(opts.Exclude, "**/generated/**", ".gitignore")
	selection := build(t, opts)
	if !reflect.DeepEqual(selection.Files, []string{"src/demo.php"}) {
		t.Fatalf("wrong full scope: %+v", selection)
	}
}

func TestPathsFromSubdirectoryAndStrictSelection(t *testing.T) {
	root := baseline(t)
	opts := options(root, "paths")
	opts.CWD = filepath.Join(root, "src")
	opts.Paths = []string{"demo.php"}
	selection := build(t, opts)
	if !reflect.DeepEqual(selection.Files, []string{"src/demo.php"}) || !selection.Contains("src/demo.php", 3) {
		t.Fatalf("caller-relative path lost: %+v", selection)
	}
	for _, name := range []string{"missing.php", "../../outside", "../.gitignore"} {
		opts.Paths = []string{name}
		if _, err := Build(context.Background(), opts); err == nil {
			t.Fatalf("invalid or out-of-root selection %q accepted", name)
		}
	}
	opts.Paths = []string{"demo.php"}
	opts.Exclude = append(opts.Exclude, "**/*.php")
	if _, err := Build(context.Background(), opts); err == nil {
		t.Fatal("explicit fully excluded path must fail")
	}
}

func TestBaseResolutionAndUnavailableHistory(t *testing.T) {
	root := baseline(t)
	opts := options(root, "diff")
	if _, err := Build(context.Background(), opts); err == nil {
		t.Fatal("must not guess main when origin/HEAD is missing")
	}
	git(t, root, "update-ref", "refs/remotes/origin/main", "main")
	git(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	selection := build(t, opts)
	if *selection.BaseRef != "refs/remotes/origin/main" || len(selection.Files) != 0 {
		t.Fatalf("bad default base: %+v", selection)
	}
	for _, ref := range []string{"missing", "--output=/tmp/not-a-ref"} {
		opts.Base = ref
		if _, err := Build(context.Background(), opts); err == nil {
			t.Fatalf("invalid base %q accepted", ref)
		}
	}
}

func TestSeparatedHunksAndLiteralTrackedNames(t *testing.T) {
	root := baseline(t)
	filename := "src/:(glob)special\nname.php"
	write(t, root, filename, "1\n2\n3\n4\n5\n")
	git(t, root, "add", "--all")
	git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "special path")
	git(t, root, "config", "diff.interHunkContext", "10")
	write(t, root, filename, "changed\n2\n3\n4\nchanged\n")
	opts := options(root, "diff")
	opts.Base = "HEAD"
	selection := build(t, opts)
	if !selection.Contains(filename, 1) || !selection.Contains(filename, 5) || selection.Contains(filename, 3) {
		t.Fatalf("literal path or zero hunk context broken: %+v", selection)
	}
}

func TestDiffUsesMergeBaseInsteadOfTipOfBaseBranch(t *testing.T) {
	root := baseline(t)
	git(t, root, "switch", "--quiet", "main")
	write(t, root, "src/demo.php", "<?php\n$changed_on_main = 1;\n")
	git(t, root, "add", "--all")
	git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "base advanced")
	git(t, root, "switch", "--quiet", "scenario")
	write(t, root, "src/feature.php", "<?php\n")
	opts := options(root, "diff")
	opts.Base = "main"
	if got := build(t, opts); !reflect.DeepEqual(got.Files, []string{"src/feature.php"}) {
		t.Fatalf("base branch-only changes leaked into scope: %+v", got)
	}
}

func TestInvalidInputsAndCancelledGit(t *testing.T) {
	root := baseline(t)
	for _, mutate := range []func(*Options){
		func(o *Options) { o.Mode = "typo" },
		func(o *Options) { o.Include = nil },
		func(o *Options) { o.Include = []string{"missing"} },
		func(o *Options) { o.Include = []string{"../outside"} },
		func(o *Options) { o.Exclude = []string{"["} },
		func(o *Options) { o.Base = "main" },
		func(o *Options) { o.WholeFiles = true },
	} {
		opts := options(root, "full")
		mutate(&opts)
		if _, err := Build(context.Background(), opts); err == nil {
			t.Fatalf("invalid options accepted: %+v", opts)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, options(root, "full")); err == nil {
		t.Fatal("cancelled Git process must not look like empty scope")
	}
}

func TestMonorepoAndWorktree(t *testing.T) {
	root := baseline(t)
	write(t, root, "apps/demo/src/site.php", "<?php\n")
	git(t, root, "add", "--all")
	git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "site")
	project := filepath.Join(root, "apps", "demo")
	selection := build(t, options(project, "full"))
	if !reflect.DeepEqual(selection.Files, []string{"src/site.php"}) {
		t.Fatalf("scope escaped monorepo project: %+v", selection)
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	git(t, root, "worktree", "add", "--quiet", "-b", "secondary", worktree)
	write(t, worktree, "src/new.php", "<?php\n")
	opts := options(worktree, "diff")
	opts.Base = "HEAD"
	if !reflect.DeepEqual(build(t, opts).Files, []string{"src/new.php"}) {
		t.Fatal("scope must work with .git file in linked worktree")
	}
}

func TestSymlinksAndNonUTF8(t *testing.T) {
	root := baseline(t)
	outside := filepath.Join(t.TempDir(), "outside.php")
	write(t, filepath.Dir(outside), "outside.php", "<?php\n")
	if err := os.Symlink(outside, filepath.Join(root, "src", "link.php")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(build(t, options(root, "full")).Files, []string{"src/demo.php"}) {
		t.Fatal("implicit scope must not follow symlinks")
	}
	opts := options(root, "paths")
	opts.Paths = []string{"src/link.php"}
	if _, err := Build(context.Background(), opts); err == nil {
		t.Fatal("explicit external symlink must fail")
	}
	write(t, root, "src/invalid\xff.php", "<?php\n")
	if _, err := Build(context.Background(), options(root, "full")); err == nil {
		t.Fatal("non-UTF-8 path must not be silently replaced in JSON")
	}
}

func TestScopeAgainstExistingScenarioFixtures(t *testing.T) {
	fixtures := filepath.Join("..", "..", "tests", "fixtures", "scenarios")
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			directory := filepath.Join(fixtures, entry.Name())
			data, err := os.ReadFile(filepath.Join(directory, "scenario.json"))
			if err != nil {
				t.Fatal(err)
			}
			var spec struct {
				State        string   `json:"state"`
				ChangedFiles []string `json:"changed_files"`
				Findings     []struct {
					Path  string   `json:"path"`
					Line  int      `json:"line"`
					Modes []string `json:"modes"`
				} `json:"findings"`
			}
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			copyTree(t, filepath.Join(directory, "before"), root)
			git(t, root, "init", "--quiet", "--initial-branch=main")
			git(t, root, "add", "--all")
			git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "baseline")
			git(t, root, "switch", "--quiet", "-c", "scenario")
			if spec.State == "untracked" {
				copyTree(t, filepath.Join(directory, "untracked"), root)
			} else {
				patch, err := filepath.Abs(filepath.Join(directory, "change.patch"))
				if err != nil {
					t.Fatal(err)
				}
				git(t, root, "apply", patch)
				if spec.State == "staged" || spec.State == "committed" {
					git(t, root, "add", "--all")
				}
				if spec.State == "committed" {
					git(t, root, "-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "change")
				}
			}
			opts := options(root, "diff")
			opts.Include, opts.Base = []string{"web/modules/custom"}, "main"
			selection := build(t, opts)
			if !reflect.DeepEqual(selection.Files, spec.ChangedFiles) {
				t.Fatalf("files=%v expected=%v", selection.Files, spec.ChangedFiles)
			}
			for _, finding := range spec.Findings {
				expected := false
				for _, mode := range finding.Modes {
					expected = expected || mode == "diff"
				}
				if selection.Contains(finding.Path, finding.Line) != expected {
					t.Fatalf("incorrect reporting scope: %+v", selection)
				}
			}
		})
	}
}

func copyTree(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(filename string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		write(t, destination, relative, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
