package agentinstructions

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rchehd/phpwarden/internal/agentskills"
)

func apply(t *testing.T, root, selection string, remove bool) []Installation {
	t.Helper()
	clients, err := agentskills.ParseClients(selection)
	if err != nil {
		t.Fatal(err)
	}
	prepare := Prepare
	if remove {
		prepare = PrepareRemoval
	}
	plan, err := prepare(context.Background(), root, clients)
	if err != nil {
		t.Fatal(err)
	}
	var rows []Installation
	if remove {
		rows, err = plan.Remove(context.Background())
	} else {
		rows, err = plan.Install(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func contents(t *testing.T, root, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestInstructionsPreserveUserBytesAndSelectiveOwnership(t *testing.T) {
	for _, original := range []string{"", "# user rules", "# user rules\n", "# user rules\r\n"} {
		t.Run(strings.ReplaceAll(original, "\n", "LF"), func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(original), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			rows := apply(t, root, "all", false)
			if len(rows) != 2 || !rows[0].Changed || !rows[1].Changed {
				t.Fatal(rows)
			}
			agents := contents(t, root, "AGENTS.md")
			claude := contents(t, root, "CLAUDE.md")
			if !bytes.HasPrefix(agents, []byte(original)) || !bytes.Contains(claude, []byte("@AGENTS.md")) {
				t.Fatal("lost project instructions or Claude import")
			}
			for _, row := range apply(t, root, "all", false) {
				if row.Changed {
					t.Fatal("repeat changed instructions")
				}
			}
			apply(t, root, "codex", true)
			b, err := owned(contents(t, root, "AGENTS.md"))
			if err != nil || !slices.Equal(b.Clients, []string{"cursor", "claude"}) {
				t.Fatalf("removed another client: %+v %v", b, err)
			}
			apply(t, root, "claude", true)
			if string(contents(t, root, "CLAUDE.md")) != original {
				t.Fatal("Claude removal lost user bytes")
			}
			apply(t, root, "cursor", true)
			if string(contents(t, root, "AGENTS.md")) != original {
				t.Fatal("shared removal lost user bytes")
			}
			info, _ := os.Stat(filepath.Join(root, "AGENTS.md"))
			if info.Mode().Perm() != 0o640 {
				t.Fatal("changed file permissions")
			}
		})
	}
}

func TestInstructionsDeleteOnlyFilesCreatedByInstaller(t *testing.T) {
	root := t.TempDir()
	apply(t, root, "all", false)
	rows := apply(t, root, "all", true)
	for _, row := range rows {
		if !row.Deleted || !row.Removed {
			t.Fatal(row)
		}
		if _, err := os.Lstat(row.Path); !os.IsNotExist(err) {
			t.Fatal("left generated-only file", err)
		}
	}
	apply(t, root, "none", false)
	apply(t, root, "all", true)
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("no-op removal created files", entries)
	}
	apply(t, root, "all", false)
	file, err := os.OpenFile(filepath.Join(root, "AGENTS.md"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("\nUser additions\n")
	file.Close()
	apply(t, root, "all", true)
	if string(contents(t, root, "AGENTS.md")) != "\nUser additions\n" {
		t.Fatal("deleted user additions")
	}
}

func TestInstructionsKeepExistingClaudeImport(t *testing.T) {
	root := t.TempDir()
	original := "# Claude\n@AGENTS.md\nUser rules\n"
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(original), 0o644)
	rows := apply(t, root, "claude", false)
	if rows[1].Changed || string(contents(t, root, "CLAUDE.md")) != original {
		t.Fatal("duplicated an existing import")
	}
	apply(t, root, "claude", true)
	if string(contents(t, root, "CLAUDE.md")) != original {
		t.Fatal("removed user-owned import")
	}
}

func TestInstructionsDoNotReuseQuotedImportExamples(t *testing.T) {
	for _, example := range []string{"```text\n@AGENTS.md\n```\n", "    @AGENTS.md\n", "~~~\n@AGENTS.md\n~~~\n", "```@AGENTS.md```\n"} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(example), 0o644)
		rows := apply(t, root, "claude", false)
		if !rows[1].Changed {
			t.Fatal("used an example as an active Claude import")
		}
		apply(t, root, "claude", true)
		if string(contents(t, root, "CLAUDE.md")) != example {
			t.Fatal("changed user example")
		}
	}
}

func TestInstructionsRejectConflictsBeforeAnyWrite(t *testing.T) {
	for _, target := range []string{"AGENTS.md", "CLAUDE.md"} {
		for _, kind := range []string{"symlink", "directory", "invalid-text", "duplicate", "customized", "oversized", "malformed", "oversized-result", "unclosed-fence"} {
			t.Run(target+"/"+kind, func(t *testing.T) {
				root := t.TempDir()
				name := filepath.Join(root, target)
				data := []byte("user data")
				switch kind {
				case "symlink":
					outside := filepath.Join(t.TempDir(), "rules.md")
					os.WriteFile(outside, data, 0o644)
					os.Symlink(outside, name)
				case "directory":
					os.Mkdir(name, 0o755)
				case "invalid-text":
					os.WriteFile(name, []byte{0xff}, 0o644)
				case "oversized":
					os.WriteFile(name, bytes.Repeat([]byte("a"), maxSize+1), 0o644)
				case "oversized-result":
					os.WriteFile(name, bytes.Repeat([]byte("a"), maxSize), 0o644)
				case "malformed":
					os.WriteFile(name, []byte(end), 0o644)
				case "unclosed-fence":
					os.WriteFile(name, []byte("# User example\n```\n@AGENTS.md\n"), 0o644)
				default:
					apply(t, root, "all", false)
					data = contents(t, root, target)
					if kind == "duplicate" {
						data = append(data, data...)
					} else {
						data = bytes.Replace(data, []byte("\n"+end), []byte("\nCustom rule\n"+end), 1)
					}
					os.WriteFile(name, data, 0o644)
				}
				beforeAgents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
				beforeClaude, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
				if _, err := Prepare(context.Background(), root, []string{"codex", "cursor", "claude"}); err == nil {
					t.Fatal("conflict accepted")
				}
				afterAgents, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
				afterClaude, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
				if !bytes.Equal(beforeAgents, afterAgents) || !bytes.Equal(beforeClaude, afterClaude) {
					t.Fatal("preflight changed instructions")
				}
			})
		}
	}
}

func TestInstructionsRecheckChangesAndSerializeClients(t *testing.T) {
	root := t.TempDir()
	plan, err := Prepare(context.Background(), root, []string{"codex"})
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Late user rules"), 0o644)
	if _, err := plan.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(contents(t, root, "AGENTS.md"), []byte("Late user rules")) {
		t.Fatal("lost concurrent user edits")
	}
	var group sync.WaitGroup
	for _, client := range []string{"codex", "cursor", "claude"} {
		group.Go(func() {
			plan, err := Prepare(context.Background(), root, []string{client})
			if err == nil {
				_, err = plan.Install(context.Background())
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	block, err := owned(contents(t, root, "AGENTS.md"))
	if err != nil || !slices.Equal(block.Clients, []string{"codex", "cursor", "claude"}) {
		t.Fatalf("client ownership lost: %+v %v", block, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := contents(t, root, "AGENTS.md")
	if _, err := plan.Remove(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if !bytes.Equal(contents(t, root, "AGENTS.md"), before) {
		t.Fatal("cancelled removal wrote instructions")
	}
}
