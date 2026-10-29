package launcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/checker/psalm"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/protocol"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func TestPsalmCapabilityIsRequiredBeforeStartingDocker(t *testing.T) {
	location, effective, options := fixture(t)
	process := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("Docker must not start without Psalm capability")
		return runner.Result{}
	})
	result := Check(context.Background(), location, effective, options, []string{"psalm"}, process)
	if result.Complete || result.ExitCode != 3 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "checker_unavailable" || !strings.Contains(result.Diagnostics[0].Message, "does not include Psalm") {
		t.Fatalf("missing capability became a pass or an opaque error: %+v", result)
	}
}

func TestPsalmTransportAcceptsWholeFlowScopeAndRejectsMalformedEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "checker", "psalm", "testdata", "drupal11-automatic.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Roots            struct{ Project, Bundle, Generated string }
		Sources, Context []string
		Result           psalmanalysis.Result
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Result.Roots = psalmanalysis.Roots(fixture.Roots)
	fixture.Result.SourceFiles, fixture.Result.ContextFiles = fixture.Sources, fixture.Context
	effective, err := config.Parse([]byte(`version: 1
profile: drupal
php: {runtime: "8.3", target: {min: "8.3", max: "8.3"}}
scope: {include: [web/modules/custom]}
tools: {psalm: {enabled: true}}
`))
	if err != nil {
		t.Fatal(err)
	}
	helper := "web/modules/custom/catalog_review/src/InputHelper.php"
	selected := scope.Selection{Mode: "diff", BaseRef: new("HEAD~2"), MergeBase: new(strings.Repeat("a", 40)), Files: []string{helper}, LineMap: map[string][]scope.Range{helper: {{Start: 999, End: 999}}}, ProjectFiles: []string{}}
	for _, variant := range []string{"valid", "policy", "trace", "fingerprint", "scope", "rule", "kind"} {
		t.Run(variant, func(t *testing.T) {
			outcome := psalm.Run(selected, effective.Config, func() psalmanalysis.Result { return fixture.Result })
			if len(outcome.Findings) != 4 {
				t.Fatalf("native fixture did not select helper flows: %+v", outcome)
			}
			request := runrequest.Request{Version: protocol.Version, RunID: "psalm-transport", Config: effective.Config, Selection: selected, Only: []string{"psalm"}, Image: testImage, Toolset: toolset.DevelopmentID("8.3"), RuntimeVersion: "8.3.36"}
			result := report.New()
			result.RunID, result.Profile, result.Scope = request.RunID, new("drupal"), &request.Selection
			result.Environment = &report.Environment{PHPRuntime: outcome.Runtime, PHPVersion: outcome.PHPVersion, RequestedRuntime: "8.3", TargetMin: "8.3", TargetMax: "8.3", Image: &request.Image, Toolset: &request.Toolset, Protocol: protocol.Version}
			result.Checks, result.Findings = []report.Check{outcome.Check}, outcome.Findings
			switch variant {
			case "policy":
				result.Checks[0].Required = false
			case "trace":
				result.Findings[0].Trace = nil
			case "fingerprint":
				result.Findings[0].Fingerprint = nil
			case "scope":
				request.Selection.Files = []string{"web/modules/custom/unrelated.php"}
				request.Selection.LineMap = map[string][]scope.Range{"web/modules/custom/unrelated.php": nil}
			case "rule":
				result.Findings[0].Rule = "psalm.UndefinedClass"
			case "kind":
				result.Findings[0].Kind = "syntax"
			}
			result.Finalize()
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeReport(runner.Result{Stdout: encoded, ExitCode: result.ExitCode}, request)
			if variant == "valid" {
				if err != nil || !decoded.Complete || len(decoded.Findings) != 4 {
					t.Fatalf("valid cross-file flows rejected: %v %+v", err, decoded)
				}
			} else if err == nil {
				t.Fatalf("invalid %s worker evidence accepted", variant)
			}
		})
	}
}
