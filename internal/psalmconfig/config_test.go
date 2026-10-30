package psalmconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/runner"
)

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return f(ctx, command)
}

func write(t *testing.T, root, name, text string) string {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestNativeDiscoveryPrecedenceAndRelativeResources(t *testing.T) {
	root := t.TempDir()
	write(t, root, "apps/storefront/psalm.xml.dist", `<psalm resolveFromConfigFile="true" errorLevel="3" errorBaseline="baseline.xml"><stubs><file name="types.php" preloadClasses="true"/></stubs></psalm>`)
	write(t, root, "apps/storefront/types.php", "<?php class ExternalType {}\n")
	baseline := `<files psalm-version="6.20.3"><file src="src.php"><TaintedHtml><code><![CDATA[$value]]></code></TaintedHtml></file></files>`
	write(t, root, "apps/storefront/baseline.xml", baseline)
	native, err := Load(root, "apps/storefront", Options{Discover: true})
	if err != nil || native.Config != "apps/storefront/psalm.xml.dist" || native.Baseline != "apps/storefront/baseline.xml" || native.stubs[0].attr("name") != "apps/storefront/types.php" || native.baseline.Children[0].attr("src") != "apps/storefront/src.php" || len(native.Inputs) != 3 {
		t.Fatalf("native relative inputs not resolved: %+v %v", native, err)
	}
	write(t, root, "psalm.xml", `<psalm errorLevel="5"/>`)
	native, err = Load(root, "apps/storefront", Options{Discover: true})
	if err != nil || native.Config != "psalm.xml" {
		t.Fatalf("repository config must win documented discovery order: %+v %v", native, err)
	}
	write(t, root, "chosen.xml", baseline)
	native, err = Load(root, "apps/storefront", Options{Config: "apps/storefront/psalm.xml.dist", Discover: false, Baseline: "chosen.xml"})
	if err != nil || native.Baseline != "chosen.xml" || native.Inputs["chosen.xml"] == "" || native.Inputs["apps/storefront/baseline.xml"] != "" {
		t.Fatalf("explicit baseline did not override native errorBaseline: %+v %v", native, err)
	}
	if native, err := Load(root, ".", Options{}); err != nil || native.Config != "" || len(native.Inputs) != 0 {
		t.Fatalf("discovery could not be disabled: %+v %v", native, err)
	}
}

func TestNativeConfigRejectsScopeExecutionAndXMLAmbiguity(t *testing.T) {
	for _, text := range []string{
		`<!DOCTYPE psalm [<!ENTITY file SYSTEM "file:///etc/passwd">]><psalm>&file;</psalm>`,
		`<?unsafe run?><psalm/>`, `<psalm/><psalm/>`, `<psalm xmlns="https://example.com"/>`,
		`<psalm errorLevel="3" errorLevel="8"/>`, `<psalm errorLevel="9"/>`, `<psalm reportMixedIssues="maybe"/>`,
		`<psalm autoloader="bootstrap.php"/>`, `<psalm runTaintAnalysis="false"/>`, `<psalm cacheDirectory="cache"/>`, `<psalm phpVersion="7.4"/>`,
		`<psalm><projectFiles><directory name="src"/></projectFiles></psalm>`, `<psalm><plugins><plugin filename="a.php"/></plugins></psalm>`,
		`<psalm xmlns:xi="http://www.w3.org/2001/XInclude"><xi:include href="other.xml"/></psalm>`,
		`<psalm><stubs><file name="../outside.php"/></stubs></psalm>`, `<psalm><stubs><file name="https://example.com/a.php"/></stubs></psalm>`,
		`<psalm><issueHandlers/><issueHandlers/></psalm>`, `<psalm><issueHandlers><UndefinedClass errorLevel="error"/><UndefinedClass errorLevel="suppress"/></issueHandlers></psalm>`,
		`<psalm><issueHandlers><UndefinedClass errorLevel="suppress"/></issueHandlers></psalm>`,
		`<psalm><issueHandlers><TaintedHtml><errorLevel type="ignore"><file name="src.php"/></errorLevel></TaintedHtml></issueHandlers></psalm>`,
		`<psalm><issueHandlers><TaintedHtml><errorLevel type="suppress"><file name="../outside.php"/></errorLevel></TaintedHtml></issueHandlers></psalm>`,
	} {
		t.Run(fmt.Sprintf("%x", text[:min(len(text), 14)]), func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "psalm.xml", text)
			if _, err := Load(root, ".", Options{Config: "psalm.xml"}); err == nil {
				t.Fatalf("accepted unsupported native config: %s", text)
			}
		})
	}
	root := t.TempDir()
	write(t, root, "psalm.xml", `<psalm xmlns="https://getpsalm.org/schema/config" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="https://getpsalm.org/schema/config https://example.com/schema.xsd" errorLevel="5"/>`)
	if _, err := Load(root, ".", Options{Config: "psalm.xml"}); err != nil {
		t.Fatalf("schema-location metadata should not require network: %v", err)
	}
	if err := os.Symlink("psalm.xml", filepath.Join(root, "linked.xml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, ".", Options{Config: "linked.xml"}); err == nil {
		t.Fatal("symlinked native config accepted")
	}
}

func TestNativeBaselineCannotSuppressReadinessOrUseAmbiguousKeys(t *testing.T) {
	for _, text := range []string{
		`<files><file src="src.php"><UndefinedClass><code>x</code></UndefinedClass></file></files>`,
		`<files><file src="../outside.php"><TaintedHtml><code>x</code></TaintedHtml></file></files>`,
		`<files><file src="@generated/other.php"><TaintedHtml><code>x</code></TaintedHtml></file></files>`,
		`<files><file src="src.php"><TaintedHtml occurrences="2"/></file></files>`,
		`<files><file src="src.php"><TaintedHtml><code> </code></TaintedHtml></file></files>`,
		`<files><file src="src.php"><TaintedHtml><code>x</code></TaintedHtml></file><file src="src.php"><TaintedHtml><code>x</code></TaintedHtml></file></files>`,
	} {
		root := t.TempDir()
		write(t, root, "baseline.xml", text)
		if _, err := Load(root, ".", Options{Baseline: "baseline.xml"}); err == nil {
			t.Fatalf("accepted ambiguous or non-taint baseline: %s", text)
		}
	}
}

func TestApplyKeepsMandatoryGraphAndSeparatesReadinessFromPolicy(t *testing.T) {
	root, run := t.TempDir(), t.TempDir()
	policy := `<psalm errorLevel="3" reportMixedIssues="false" errorBaseline="baseline.xml"><stubs><file name="types.php"/></stubs><issueHandlers><TaintedHtml><errorLevel type="suppress"><file name="src.php"/></errorLevel></TaintedHtml></issueHandlers></psalm>`
	write(t, root, "psalm.xml", policy)
	write(t, root, "types.php", "<?php class AddedType {}\n")
	write(t, root, "baseline.xml", `<files psalm-version="6.20.3"><file src="src.php"><TaintedHtml><code><![CDATA[$input]]></code></TaintedHtml></file><file src="@generated/entrypoints.php"><TaintedHtml><code>$build</code></TaintedHtml></file></files>`)
	profile := `<psalm errorLevel="6" resolveFromConfigFile="true" cacheDirectory="/private/cache" autoloader="/private/bootstrap.php"><projectFiles><file name="/project/src.php"/><file name="/private/entrypoints.php"/></projectFiles><stubs><file name="/private/instances.php"/></stubs><plugins><pluginClass class="MandatoryPlugin"/></plugins></psalm>`
	config := write(t, run, "prepared/psalm.xml", profile)
	native, err := Load(root, ".", Options{Discover: true})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	prepared, err := native.Apply(context.Background(), root, "/tool-vendor", run, "php", 1536, psalmanalysis.Prepared{Config: config, Files: []string{"src.php"}, Inputs: map[string]string{}}, processFunc(func(ctx context.Context, command runner.Command) runner.Result {
		called = true
		var files []string
		if json.Unmarshal(command.Input, &files) != nil || len(files) != 2 {
			t.Fatal("schema validation did not receive both configurations")
		}
		return runner.Result{Stdout: []byte(`{"version":1,"checked":2,"errors":[]}`)}
	}))
	if err != nil || !called || len(prepared.SyntaxFiles) != 1 || prepared.SyntaxFiles[0] != "types.php" || len(prepared.Inputs) != 3 {
		t.Fatalf("native configuration failed: %+v %v", prepared, err)
	}
	readinessBytes, _ := os.ReadFile(prepared.ReadinessConfig)
	taintBytes, _ := os.ReadFile(prepared.Config)
	readiness, _ := parse(readinessBytes)
	taint, _ := parse(taintBytes)
	if readiness.attr("errorLevel") != "3" || readiness.attr("errorBaseline") != "" || strings.Contains(string(readinessBytes), "issueHandlers") || taint.attr("errorLevel") != "3" || taint.attr("errorBaseline") == "" {
		t.Fatal("report policy leaked into readiness")
	}
	for _, data := range [][]byte{readinessBytes, taintBytes} {
		for _, required := range []string{"MandatoryPlugin", "/private/instances.php", "/private/entrypoints.php", filepath.Join(root, "types.php")} {
			if !strings.Contains(string(data), required) {
				t.Fatalf("mandatory model or project metadata lost: %s", data)
			}
		}
	}
	if filepath.Dir(prepared.Config) != filepath.Dir(config) || filepath.Dir(prepared.ReadinessConfig) != filepath.Dir(config) {
		t.Fatal("native file-name base changed")
	}
	data, _ := os.ReadFile(taint.attr("errorBaseline"))
	baseline, _ := parse(data)
	want, _ := filepath.Rel(filepath.Dir(config), filepath.Join(root, "src.php"))
	if baseline.Children[0].attr("src") != filepath.ToSlash(want) || baseline.Children[1].attr("src") != "entrypoints.php" {
		t.Fatalf("baseline paths not rebased to native file-name base: %s", data)
	}
	original, _ := os.ReadFile(filepath.Join(root, "psalm.xml"))
	if string(original) != policy {
		t.Fatal("project native config was modified")
	}
	write(t, root, "types.php", "<?php class ChangedType {}\n")
	if _, err := native.Apply(context.Background(), root, "/tool-vendor", run, "php", 1536, psalmanalysis.Prepared{Config: config}, nil); err == nil {
		t.Fatal("stale native stub accepted")
	}
}

func TestApplyRejectsPartialNativeValidation(t *testing.T) {
	for _, variant := range []string{"invalid-json", "wrong-count", "stderr", "exit", "error", "schema"} {
		t.Run(variant, func(t *testing.T) {
			root, run := t.TempDir(), t.TempDir()
			write(t, root, "psalm.xml", `<psalm errorLevel="5"/>`)
			config := write(t, run, "prepared/psalm.xml", `<psalm resolveFromConfigFile="true" errorLevel="6"/>`)
			native, err := Load(root, ".", Options{Discover: true})
			if err != nil {
				t.Fatal(err)
			}
			_, err = native.Apply(context.Background(), root, "/bundle", run, "php", 512, psalmanalysis.Prepared{Config: config}, processFunc(func(context.Context, runner.Command) runner.Result {
				result := runner.Result{Stdout: []byte(`{"version":1,"checked":2,"errors":[]}`)}
				switch variant {
				case "invalid-json":
					result.Stdout = []byte(`{`)
				case "wrong-count":
					result.Stdout = []byte(`{"version":1,"checked":1,"errors":[]}`)
				case "stderr":
					result.Stderr = []byte("warning")
				case "exit":
					result.ExitCode = 3
				case "error":
					result.Err = fmt.Errorf("process failed")
				case "schema":
					result.ExitCode = 3
					result.Stdout = []byte(`{"version":1,"checked":2,"errors":["unsupported native setting"]}`)
				}
				return result
			}))
			if err == nil {
				t.Fatal("partial validation accepted")
			}
			if variant == "schema" && !strings.Contains(err.Error(), "unsupported native setting") {
				t.Fatal("schema diagnostic lost")
			}
		})
	}
}
