package semgrep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const customRule = "rules:\n  - id: custom.echo\n    languages: [php]\n    message: Avoid this echo\n    severity: INFO\n    pattern: echo 42;\n"

func TestLocalRulesDiscoverySelectionAndContext(t *testing.T) {
	root := t.TempDir()
	cfg := settings(t)
	before := ContextHash(root, "platform", cfg)
	write(t, root, "platform/.semgrep.yaml", customRule)
	prepared, err := prepare(root, "platform", cfg)
	if err != nil || len(prepared.Rules) != 5 || ContextHash(root, "platform", cfg) == before {
		t.Fatalf("local rules discovery: %+v, %v", prepared, err)
	}
	cfg.Rules = []string{"custom.echo"}
	prepared, err = prepare(root, "platform", cfg)
	if err != nil || len(prepared.Rules) != 1 || prepared.Rules[0].ID != "custom.echo" {
		t.Fatalf("rule selection: %+v, %v", prepared, err)
	}
	write(t, root, ".semgrep.yml", strings.ReplaceAll(customRule, "custom.echo", "nearer.echo"))
	if _, err := prepare(root, "platform", cfg); err == nil {
		t.Fatal("changed discovery priority was ignored")
	}
	cfg.Config = "platform/.semgrep.yaml"
	cfg.ExcludeRules = []string{"custom.echo"}
	prepared, err = prepare(root, "platform", cfg)
	if err != nil || len(prepared.Rules) != 0 {
		t.Fatalf("explicit rule exclusion: %+v, %v", prepared, err)
	}
}

func TestUnsafeOrUnsupportedRulesAndBaselineAreRejected(t *testing.T) {
	for _, replacement := range []string{
		"    validators: []\n", "    paths: {include: ['*.php']}\n", "    options: {interfile: true}\n", "    pattern-where-python: dangerous()\n", "    fix: replacement\n",
	} {
		if _, err := parseRules([]byte(customRule + replacement)); err == nil {
			t.Fatalf("unsupported feature accepted: %s", replacement)
		}
	}
	for _, source := range []string{strings.ReplaceAll(customRule, "[php]", "[python]"), customRule + "---\nrules: []\n", strings.ReplaceAll(customRule, "    pattern:", "    id: duplicate\n    pattern:")} {
		if _, err := parseRules([]byte(source)); err == nil {
			t.Fatal("invalid rules document accepted")
		}
	}
	root := t.TempDir()
	cfg := settings(t)
	cfg.Config = "rules.yml"
	write(t, root, "rules.yml", customRule)
	if err := os.Symlink("rules.yml", filepath.Join(root, "link.yml")); err != nil {
		t.Fatal(err)
	}
	cfg.Config = "link.yml"
	if _, err := prepare(root, ".", cfg); err == nil {
		t.Fatal("rules symlink accepted")
	}
	cfg.Config = "rules.yml"
	cfg.Baseline = "baseline.json"
	write(t, root, "baseline.json", `{"version":1,"fingerprints":["invalid"]}`)
	if _, err := prepare(root, ".", cfg); err == nil {
		t.Fatal("malformed baseline accepted")
	}
}
