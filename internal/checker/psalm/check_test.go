package psalm

import (
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
)

func configured(t *testing.T) config.Config {
	t.Helper()
	effective, err := config.Parse([]byte(`version: 1
profile: drupal
php: {runtime: "8.3", target: {min: "8.3", max: "8.3"}}
scope: {include: [web/modules/custom]}
tools: {psalm: {enabled: true}}
`))
	if err != nil {
		t.Fatal(err)
	}
	return effective.Config
}

func TestPublicLifecycleSkipsBeforeNativeWorkAndRejectsRuntimeFallback(t *testing.T) {
	for _, mode := range []string{"disabled", "empty", "unrelated", "tests", "runtime"} {
		t.Run(mode, func(t *testing.T) {
			cfg := configured(t)
			selection := selected("web/modules/custom/demo/demo.module")
			wantStatus, wantReason := "skipped", "no_files"
			switch mode {
			case "disabled":
				cfg.Tools.Psalm.Enabled, wantReason = false, "disabled"
			case "empty":
				selection = selected()
			case "unrelated":
				selection = selected("script.js")
			case "tests":
				selection = selected("web/modules/custom/demo/tests/Test.php")
			case "runtime":
				cfg.PHP.Runtime, wantStatus, wantReason = "8.4", "unavailable", "runtime_unsupported"
			}
			outcome := Run(selection, cfg, func() psalmanalysis.Result { t.Fatal("native work must not start"); return psalmanalysis.Result{} })
			if outcome.Check.Status != wantStatus || outcome.Check.Reason == nil || *outcome.Check.Reason != wantReason {
				t.Fatalf("unexpected lifecycle: %+v", outcome)
			}
		})
	}
	cfg := configured(t)
	cfg.Tools.Psalm.Enabled = false
	if !InputRequired(cfg, selected("demo.php"), []string{"psalm"}) || InputRequired(cfg, selected("demo.php"), nil) {
		t.Fatal("--only must enable Psalm for that run")
	}
	cfg.Tools.Psalm.Enabled = true
	if InputRequired(cfg, selected("demo.php"), []string{"debug"}) {
		t.Fatal("another --only selection must exclude Psalm")
	}
}

func TestReportFiltersKeepTheAnalysisGraphAndConstrainMetadataExpansion(t *testing.T) {
	root := "web/modules/custom/catalog_review/"
	for _, tc := range []struct {
		name             string
		include, exclude []string
		want             int
	}{
		{"all", nil, nil, 7},
		{"source helper only", []string{root + "src/InputHelper.php"}, nil, 4},
		{"form only", []string{root + "src/Form"}, nil, 1},
		{"controller only", []string{root + "src/Controller"}, nil, 5},
		{"excluded target", []string{root + "src/Form"}, []string{"**/Form/**"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			native := captured(t, "drupal11-automatic")
			cfg := configured(t)
			cfg.Tools.Psalm.Include, cfg.Tools.Psalm.Exclude = tc.include, tc.exclude
			selection := selected()
			selection.ProjectFiles = []string{"composer.lock"}
			called := false
			outcome := Run(selection, cfg, func() psalmanalysis.Result { called = true; return native })
			if !called || outcome.Check.Status != "passed" || len(outcome.Findings) != tc.want || len(native.Findings) != 7 || len(native.SourceFiles) != 4 {
				t.Fatalf("report filtering changed coverage or policy: %+v", outcome)
			}
			for _, finding := range outcome.Findings {
				if !ValidFinding(finding, selection, cfg) {
					t.Fatalf("launcher rejected a valid metadata-selected flow: %+v", finding)
				}
			}
		})
	}
}

func TestLauncherFlowValidationIncludesIntermediateFilesWithoutLineFiltering(t *testing.T) {
	native := captured(t, "drupal11-monorepo")
	cfg := configured(t)
	cfg.Scope.Include = []string{"apps/storefront/web/modules/custom", "packages/outside_module"}
	for _, name := range native.SourceFiles {
		if !strings.HasSuffix(name, "/src/InputHelper.php") {
			continue
		}
		selection := selected(name)
		outcome := Run(selection, cfg, func() psalmanalysis.Result { return native })
		if len(outcome.Findings) != 4 {
			t.Fatalf("intermediate flow selection lost: %+v", outcome)
		}
		for _, finding := range outcome.Findings {
			if !ValidFinding(finding, selection, cfg) || ValidFinding(finding, selected("packages/outside_module/unrelated.php"), cfg) {
				t.Fatal("launcher did not use whole-file flow relevance")
			}
			invalid := finding
			invalid.Trace = nil
			if ValidFinding(invalid, selection, cfg) {
				t.Fatal("trace-free finding accepted")
			}
			invalid = finding
			invalid.Fingerprint = nil
			if ValidFinding(invalid, selection, cfg) {
				t.Fatal("fingerprint-free finding accepted")
			}
		}
		return
	}
	t.Fatal("native monorepo fixture lacks helper")
}

func TestAdvisoryNeverHidesExecutionFailure(t *testing.T) {
	cfg := configured(t)
	for _, required := range []bool{false, true} {
		cfg.Tools.Psalm.Required = required
		outcome := Run(selected("web/modules/custom/demo/demo.module"), cfg, func() psalmanalysis.Result {
			return psalmanalysis.Result{Diagnostics: []psalmanalysis.Diagnostic{{Stage: "dependencies", Code: "dependencies_missing", Message: "Missing pinned bundle"}}}
		})
		result := envelope(outcome.Findings, outcome.Check)
		if result.Complete || required && result.ExitCode != 3 || !required && result.ExitCode != 0 || len(outcome.Findings) != 0 {
			t.Fatalf("execution failure hidden: %+v", outcome)
		}
	}
}
