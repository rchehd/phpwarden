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
				if len(result.Paths) != len(Names) {
					t.Fatalf("not all skills installed: %+v", result.Paths)
				}
				for _, name := range Names {
					path := filepath.Join(root, ".agents/skills", name)
					if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
						t.Fatal(err)
					}
					if slices.Contains(clients, "claude") {
						resolved, err := filepath.EvalSymlinks(filepath.Join(root, ".claude/skills", name))
						if err != nil || resolved != path {
							t.Fatalf("%s alias: %s %v", name, resolved, err)
						}
					}
				}
				for _, base := range []string{".agents/skills", ".claude/skills"} {
					if base == ".claude/skills" && !slices.Contains(clients, "claude") {
						continue
					}
					for _, resource := range []string{"phpwarden/references/scope.md", "phpwarden-review/references/checklist.md", "phpwarden-review/references/standards.md", "phpwarden-review/assets/review-checklist.md", "phpwarden-tests/references/reports.md", "phpwarden-report/references/evidence.md", "phpwarden-report/assets/report-template.md"} {
						data, err := os.ReadFile(filepath.Join(root, base, resource))
						want, _ := content.ReadFile("skills/" + resource)
						if err != nil || !bytes.Equal(data, want) {
							t.Fatalf("skill resource %s missing or corrupt through %s: %v", resource, base, err)
						}
					}
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
	for _, name := range []string{
		".agents/skills/" + Name + "/SKILL.md", ".agents/skills/" + Name + "/references/deptrac.md", ".claude/skills/" + Name,
		".agents/skills/phpwarden-tests/SKILL.md", ".agents/skills/phpwarden-tests/references/reports.md", ".claude/skills/phpwarden-tests",
		".agents/skills/phpwarden-report/SKILL.md", ".agents/skills/phpwarden-report/references/evidence.md", ".agents/skills/phpwarden-report/assets/report-template.md", ".claude/skills/phpwarden-report",
		".agents/skills/phpwarden/SKILL.md", ".agents/skills/phpwarden/references/scope.md", ".claude/skills/phpwarden",
		".agents/skills/phpwarden-review/SKILL.md", ".agents/skills/phpwarden-review/references/checklist.md", ".agents/skills/phpwarden-review/references/standards.md", ".agents/skills/phpwarden-review/assets/review-checklist.md",
	} {
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

func TestInstallAddsMissingSkillToExistingBundle(t *testing.T) {
	for _, missing := range []string{EntryName, "phpwarden-tests", "phpwarden-report"} {
		t.Run(missing, func(t *testing.T) {
			testBundleExtension(t, missing)
		})
	}
}

func testBundleExtension(t *testing.T, missing string) {
	t.Helper()
	root := t.TempDir()
	plan, err := Prepare(root, clients)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Install(); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(root, ".agents/skills", Name, "SKILL.md")
	before, _ := os.ReadFile(oldPath)
	if err := os.RemoveAll(filepath.Join(root, ".agents/skills", missing)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".claude/skills", missing)); err != nil {
		t.Fatal(err)
	}
	plan, err = Prepare(root, clients)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Install()
	if err != nil || !result.Created || !result.ClaudeCreated {
		t.Fatalf("bundle extension: %+v %v", result, err)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing skill changed")
	}
	if _, err := os.ReadFile(filepath.Join(root, ".claude/skills/phpwarden-tests/references/reports.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(root, ".claude/skills/phpwarden-report/assets/report-template.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(root, ".claude/skills/phpwarden/references/scope.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSkillInstallationPreservesProjectCriteriaAndInstructions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "phpwarden-review.md", ".phpwarden.yaml"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("project-owned "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := Prepare(root, clients)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Install(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "phpwarden-review.md", ".phpwarden.yaml"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != "project-owned "+name {
			t.Fatalf("project file changed: %s: %v", name, err)
		}
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
