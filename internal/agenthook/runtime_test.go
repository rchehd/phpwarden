package agenthook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
)

func TestClientProtocolsAndWorkspaceSelection(t *testing.T) {
	for _, client := range []string{"codex", "claude", "cursor"} {
		name := "Stop"
		if client == "cursor" {
			name = "stop"
		}
		data := `{"session_id":"s","conversation_id":"c","hook_event_name":"` + name + `","workspace_roots":["/project"],"cwd":"/project","extra":"accepted"}`
		event, err := Read(strings.NewReader(data), client, "/fallback", "")
		if err != nil || event.CWD != "/project" {
			t.Fatalf("%+v %v", event, err)
		}
		result := Feedback(client, "fix issues", true)
		if client == "cursor" && (result.Followup == "" || result.Decision != "") || client != "cursor" && (result.Decision != "block" || result.Reason == "") {
			t.Fatalf("wrong protocol %+v", result)
		}
		encoded, _ := json.Marshal(Feedback(client, "warning", false))
		if !json.Valid(encoded) {
			t.Fatal("invalid hook JSON")
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"hook_event_name":"wrong"}`, `{"hook_event_name":"Stop","cwd":"relative"}`, `{"hook_event_name":"stop","workspace_roots":["/a","/b"]}`, strings.Repeat(" ", (1<<20)+1)} {
		if _, err := Read(strings.NewReader(raw), "cursor", "/fallback", ""); err == nil {
			t.Fatalf("accepted invalid event %.100s", raw)
		}
	}
	event, err := Read(strings.NewReader(`{"hook_event_name":"stop","workspace_roots":["/a","/b"]}`), "cursor", "/fallback", "/chosen")
	if err != nil || event.CWD != "/chosen" {
		t.Fatalf("explicit multi-root selection: %+v %v", event, err)
	}
}

func TestBoundedRetriesAndFreshUserTurn(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	settings := config.Completion{Enabled: true, MaxAttempts: 2, TimeoutSeconds: 5}
	event := Event{SessionID: "session"}
	calls := 0
	check := func(context.Context) report.Report {
		calls++
		result := report.New()
		result.ExitCode = 1
		result.Verdict = "fail"
		return result
	}
	for attempt := range 4 {
		var stderr bytes.Buffer
		result := Run(context.Background(), root, "codex", event, settings, check, &stderr)
		if (result.Decision == "block") != (attempt < 2) {
			t.Fatalf("attempt %d: %+v", attempt, result)
		}
		if attempt >= 2 && (!strings.Contains(result.Message, "stopped automatic retries") || stderr.Len() == 0) {
			t.Fatal("retry limit hid unresolved errors")
		}
		event.Active = true
	}
	event.Active = false
	if result := Run(context.Background(), root, "codex", event, settings, check, io.Discard); result.Decision != "block" {
		t.Fatal("fresh user turn did not reset retry budget")
	}
	if calls != 5 {
		t.Fatal("hook reused stale result")
	}
}

func TestPolicyAbortReentryAndOptionalCoverage(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	settings := config.Completion{Enabled: true, MaxAttempts: 2, TimeoutSeconds: 5}
	for _, code := range []int{0, 1, 2, 3, 130} {
		for _, client := range []string{"codex", "cursor", "claude"} {
			result := Run(context.Background(), root, client, Event{SessionID: "s"}, settings, func(context.Context) report.Report { r := report.New(); r.ExitCode = code; return r }, io.Discard)
			if (result.Decision == "block" || result.Followup != "") != (code > 0 && code != 130) {
				t.Fatalf("exit %d client %s: %+v", code, client, result)
			}
		}
	}
	var stderr bytes.Buffer
	optional := Run(context.Background(), root, "codex", Event{SessionID: "s"}, settings, func(context.Context) report.Report { r := report.New(); r.Complete = false; return r }, &stderr)
	if optional.Decision != "" || !strings.Contains(optional.Message, "incomplete") || stderr.Len() == 0 {
		t.Fatalf("optional failure policy: %+v", optional)
	}
	check := func(context.Context) report.Report { t.Fatal("bypassed event ran checks"); return report.New() }
	for _, status := range []string{"aborted", "error"} {
		Run(context.Background(), root, "cursor", Event{Status: status}, settings, check, io.Discard)
	}
	settings.Enabled = false
	Run(context.Background(), root, "codex", Event{}, settings, check, io.Discard)
	settings.Enabled = true
	t.Setenv("PHPWARDEN_COMPLETION_ACTIVE", "1")
	Run(context.Background(), root, "codex", Event{}, settings, check, io.Discard)
}

func TestProjectChecksSerializeAndCancel(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	settings := config.Completion{Enabled: true, MaxAttempts: 2, TimeoutSeconds: 5}
	var active atomic.Int32
	check := func(context.Context) report.Report {
		if active.Add(1) != 1 {
			t.Error("concurrent checks on same project")
		}
		time.Sleep(15 * time.Millisecond)
		active.Add(-1)
		return report.New()
	}
	var group sync.WaitGroup
	for _, client := range []string{"codex", "cursor", "claude"} {
		group.Go(func() { Run(context.Background(), root, client, Event{SessionID: "s"}, settings, check, io.Discard) })
	}
	group.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Run(ctx, root, "codex", Event{SessionID: "s"}, settings, check, io.Discard)
	if result != (Decision{}) {
		t.Fatalf("cancellation continued agent: %+v", result)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result = Run(ctx, root, "claude", Event{SessionID: "s", Active: true}, settings, func(ctx context.Context) report.Report { <-ctx.Done(); return report.New() }, io.Discard)
	if result.Decision != "" || !strings.Contains(result.Message, "could not finish") {
		t.Fatalf("timeout loop: %+v", result)
	}
}
