package config

import "testing"

func TestPsalmOptInDefaultsAndStrictSettings(t *testing.T) {
	for _, suffix := range []string{"", "tools: {psalm: {}}\n"} {
		effective, err := Parse([]byte(minimal + suffix))
		if err != nil {
			t.Fatal(err)
		}
		cfg := effective.Config.Tools.Psalm
		if cfg.Enabled || !cfg.Required || cfg.FailOn != "never" || cfg.TimeoutSeconds != 300 || cfg.MemoryLimitMB != 1536 || effective.Provenance["tools.psalm.enabled"] != "core_default" {
			t.Fatalf("unexpected defaults: %+v", effective)
		}
	}
	effective, err := Parse([]byte(minimal + `tools:
  psalm:
    enabled: true
    required: false
    fail_on: warning
    timeout_seconds: 180
    memory_limit_mb: 2048
    config: quality/psalm.xml
    baseline: quality/psalm-taint.xml
    discover_config: false
    include: [web/modules/custom/demo]
    exclude: ["**/generated/**"]
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := effective.Config.Tools.Psalm
	if !cfg.Enabled || cfg.Required || cfg.FailOn != "warning" || cfg.TimeoutSeconds != 180 || cfg.MemoryLimitMB != 2048 || len(cfg.Include) != 1 || len(cfg.Exclude) != 1 || cfg.DiscoverConfig || cfg.Config != "quality/psalm.xml" || cfg.Baseline != "quality/psalm-taint.xml" || effective.Provenance["tools.psalm.required"] != "project_config" {
		t.Fatalf("overrides lost: %+v", effective)
	}
	for _, setting := range []string{"enabled: yes", "unknown: true", "timeout_seconds: 0", "memory_limit_mb: 127", "fail_on: critical", "include: [../outside]", "exclude: [/outside]", "include: [src, src]", "config: auto", "config: 'https://example.com/config.xml'", "baseline: /outside.xml"} {
		if _, err := Parse([]byte(minimal + "tools: {psalm: {" + setting + "}}\n")); err == nil {
			t.Errorf("accepted unsupported or unsafe setting: %s", setting)
		}
	}
}
