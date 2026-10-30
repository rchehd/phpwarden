package projectinit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
)

func TestInitAgentInstallationPreservesExistingConfiguration(t *testing.T) {
	root := drupalProject(t, ">=8.1", nil)
	options := Options{CWD: root, Runtime: "8.4", Agents: []string{"codex", "cursor", "claude"}}
	result, err := Run(context.Background(), options)
	if err != nil || !result.Created || result.AgentSkills == nil || !result.AgentSkills.Created {
		t.Fatalf("new project integration: %+v %v", result, err)
	}
	before, _ := os.ReadFile(result.Location.Path)
	result, err = Run(context.Background(), options)
	after, _ := os.ReadFile(result.Location.Path)
	if err != nil || result.Created || result.AgentSkills == nil || result.AgentSkills.Created || string(before) != string(after) {
		t.Fatalf("repeated integration changed config or skill: %+v %v", result, err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude/settings.json")); !os.IsNotExist(err) {
		t.Fatal("skill installation changed harness settings")
	}
}

func TestInitMonorepoAgentSkillsUseGitRoot(t *testing.T) {
	root := t.TempDir()
	composer := drupalProject(t, ">=8.1", nil)
	if err := os.Rename(composer, filepath.Join(root, "platform")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".ddev/config.yaml", "php_version: \"8.3\"\n")
	oldConfig := "version: 1\nprofile: drupal\nphp: {runtime: '8.3', target: {min: '8.3', max: '8.3'}}\n"
	writeFile(t, root, "platform/.phpwarden.yaml", oldConfig)
	gitRepository(t, root)
	for attempt := range 2 {
		cwd := filepath.Join(root, "platform/web/modules/custom")
		if attempt > 0 {
			cwd = root
		}
		result, err := Run(context.Background(), Options{CWD: cwd, Agents: []string{"codex", "cursor", "claude"}})
		if err != nil || result.Created != (attempt == 0) || result.Location.Root != root || result.AgentSkills == nil || result.AgentSkills.Created != (attempt == 0) {
			t.Fatalf("monorepo installation: %+v %v", result, err)
		}
		effective, err := config.Load(result.Location)
		if err != nil || effective.Config.ComposerRoot != "platform" {
			t.Fatalf("monorepo Composer root lost: %+v %v", effective, err)
		}
		skillPath := filepath.Join(root, ".agents/skills/phpwarden-architecture")
		link, err := filepath.EvalSymlinks(filepath.Join(root, ".claude/skills/phpwarden-architecture"))
		if err != nil || link != skillPath || result.AgentSkills.Path != skillPath {
			t.Fatalf("monorepo alias points at the wrong root: %+v %s %v", result, link, err)
		}
		ref := filepath.Join(root, ".claude/skills/phpwarden-tests/references/reports.md")
		resolved, err := filepath.EvalSymlinks(ref)
		if err != nil || resolved != filepath.Join(root, ".agents/skills/phpwarden-tests/references/reports.md") {
			t.Fatalf("test quality reference uses wrong project root: %s %v", resolved, err)
		}
		ref = filepath.Join(root, ".claude/skills/phpwarden-report/assets/report-template.md")
		resolved, err = filepath.EvalSymlinks(ref)
		if err != nil || resolved != filepath.Join(root, ".agents/skills/phpwarden-report/assets/report-template.md") {
			t.Fatalf("Markdown report template uses wrong project root: %s %v", resolved, err)
		}
		if _, err := os.Lstat(filepath.Join(root, "platform/.agents")); !os.IsNotExist(err) {
			t.Fatal("skills installed under Composer root")
		}
		if data, _ := os.ReadFile(filepath.Join(root, "platform/.phpwarden.yaml")); string(data) != oldConfig {
			t.Fatal("preexisting nested configuration was overwritten")
		}
	}
}

func TestInitAgentConflictAndCancellationWriteNoConfiguration(t *testing.T) {
	for _, conflict := range []string{"alias", "reference", "report-template", "report-alias", "cancel"} {
		root := drupalProject(t, ">=8.1", nil)
		options := Options{CWD: root, Runtime: "8.4", Agents: []string{"claude"}}
		cancel := conflict == "cancel"
		if cancel {
			options.SelectAgents = func() ([]string, error) { return nil, context.Canceled }
		} else if conflict == "reference" {
			writeFile(t, root, ".agents/skills/phpwarden-tests/references/reports.md", "User's reference")
		} else if conflict == "report-template" {
			writeFile(t, root, ".agents/skills/phpwarden-report/assets/report-template.md", "User's template")
		} else if conflict == "report-alias" {
			writeFile(t, root, ".claude/skills/phpwarden-report/SKILL.md", "User's report skill")
		} else {
			writeFile(t, root, ".claude/skills/phpwarden-architecture/SKILL.md", "User's skill")
		}
		_, err := Run(context.Background(), options)
		if err == nil || cancel && !errors.Is(err, context.Canceled) {
			t.Fatalf("invalid integration succeeded: %v", err)
		}
		for _, name := range []string{".phpwarden.yaml", ".agents/skills/phpwarden-architecture"} {
			if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
				t.Fatalf("rejected init wrote %s", name)
			}
		}
	}
}
