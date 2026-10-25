package agentskills

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

func TestSharedSkillAndClaudeLinkAreIdempotent(t *testing.T) {
	for _, selection := range []string{"all", "codex", "cursor", "claude", "codex,cursor", "none"} {
		t.Run(selection, func(t *testing.T) {
			root := t.TempDir()
			clients, err := ParseClients(selection)
			if err != nil {
				t.Fatal(err)
			}
			for attempt := range 2 {
				plan, err := Prepare(root, clients)
				if err != nil {
					t.Fatal(err)
				}
				result, err := plan.Install()
				if err != nil {
					t.Fatal(err)
				}
				if selection == "none" {
					if result != nil {
						t.Fatal("none installed skills")
					}
					entries, _ := os.ReadDir(root)
					if len(entries) != 0 {
						t.Fatal("none wrote project files")
					}
					continue
				}
				if result.Created != (attempt == 0) || !slices.Equal(result.Clients, clients) {
					t.Fatalf("installation state: %+v", result)
				}
				data, err := os.ReadFile(filepath.Join(result.Path, "SKILL.md"))
				want, _ := content.ReadFile("skills/" + Name + "/SKILL.md")
				if err != nil || !bytes.Equal(data, want) {
					t.Fatal("shared skill missing or corrupt")
				}
				if _, err := os.Stat(filepath.Join(result.Path, "references/deptrac.md")); err != nil {
					t.Fatal(err)
				}
				if slices.Contains(clients, "claude") {
					resolved, err := filepath.EvalSymlinks(result.ClaudePath)
					if err != nil || resolved != result.Path || result.ClaudeCreated != (attempt == 0) {
						t.Fatalf("Claude link: %+v %s %v", result, resolved, err)
					}
				} else if _, err := os.Lstat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
					t.Fatal("unselected Claude integration was written")
				}
				if _, err := os.Lstat(filepath.Join(root, ".cursor")); !os.IsNotExist(err) {
					t.Fatal("Cursor did not reuse shared skills")
				}
			}
		})
	}
}

func TestForeignSkillsAndSymlinkParentsArePreserved(t *testing.T) {
	for _, name := range []string{".agents", ".agents/skills", ".agents/skills/" + Name, ".claude", ".claude/skills"} {
		t.Run(name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			filename := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filename); err != nil {
				t.Fatal(err)
			}
			if _, err := Prepare(root, clients); err == nil {
				t.Fatal("symlink parent accepted")
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 0 {
				t.Fatal("installation escaped project")
			}
		})
	}
	for _, name := range []string{".agents/skills/" + Name + "/SKILL.md", ".agents/skills/" + Name + "/references/deptrac.md", ".claude/skills/" + Name} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			filename := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, []byte("user content"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Prepare(root, clients); err == nil {
				t.Fatal("foreign skill accepted")
			}
			data, _ := os.ReadFile(filename)
			if string(data) != "user content" {
				t.Fatal("foreign content overwritten")
			}
		})
	}
}

func TestConcurrentInstallationsPublishCompleteFiles(t *testing.T) {
	root := t.TempDir()
	plan, err := Prepare(root, clients)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			if _, err := plan.Install(); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if _, err := Prepare(root, clients); err != nil {
		t.Fatal(err)
	}
}

func TestClientSelectionRejectsUnknownAndDuplicateClients(t *testing.T) {
	for _, value := range []string{"all,codex", "none,claude", "codex,codex", "other", "codex,", "codex, cursor"} {
		if _, err := ParseClients(value); err == nil {
			t.Fatalf("invalid client selection %q accepted", value)
		}
	}
}
