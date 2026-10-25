package agenthook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/textreport"
)

type Event struct {
	SessionID      string   `json:"session_id"`
	ConversationID string   `json:"conversation_id"`
	CWD            string   `json:"cwd"`
	WorkspaceRoots []string `json:"workspace_roots"`
	Name           string   `json:"hook_event_name"`
	Status         string   `json:"status"`
	Active         bool     `json:"stop_hook_active"`
	LoopCount      int      `json:"loop_count"`
}

type Decision struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Followup string `json:"followup_message,omitempty"`
	Message  string `json:"systemMessage,omitempty"`
}

type state struct {
	Attempts int       `json:"attempts"`
	Updated  time.Time `json:"updated"`
}

func Read(input io.Reader, client, cwd, project string) (Event, error) {
	data, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || !json.Valid(data) || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return Event{}, fmt.Errorf("invalid or oversized completion hook input")
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, err
	}
	expected := "Stop"
	if client == "cursor" {
		expected = "stop"
	}
	if event.Name != expected || event.LoopCount < 0 {
		return Event{}, fmt.Errorf("expected %s hook input", expected)
	}
	if project != "" {
		event.CWD = project
	}
	if event.CWD == "" && client == "cursor" {
		if len(event.WorkspaceRoots) != 1 {
			return Event{}, fmt.Errorf("Cursor completion requires one workspace root or an explicit --project")
		}
		event.CWD = event.WorkspaceRoots[0]
	}
	if event.CWD == "" {
		event.CWD = cwd
	}
	if !filepath.IsAbs(event.CWD) {
		return Event{}, fmt.Errorf("completion hook cwd must be absolute")
	}
	return event, nil
}

func Feedback(client, message string, block bool) Decision {
	if client == "cursor" {
		if block {
			return Decision{Followup: message}
		}
		return Decision{}
	}
	if block {
		return Decision{Decision: "block", Reason: message}
	}
	return Decision{Message: message}
}

func Run(ctx context.Context, root, client string, event Event, settings config.Completion, check func(context.Context) report.Report, output io.Writer) Decision {
	if !settings.Enabled || event.Status == "aborted" || event.Status == "error" || os.Getenv("PHPWARDEN_COMPLETION_ACTIVE") == "1" {
		return Decision{}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(settings.TimeoutSeconds)*time.Second)
	defer cancel()
	problem := func(err error) Decision {
		message := "PHPWarden completion check could not finish: " + err.Error() + ". Use phpwarden doctor; report the incomplete check to the user."
		fmt.Fprintln(output, message)
		if ctx.Err() == context.Canceled {
			return Decision{}
		}
		return Feedback(client, message, !event.Active && event.LoopCount == 0)
	}
	directory, err := cache.Directory("completion")
	if err != nil {
		return problem(err)
	}
	store, err := cache.Open(directory)
	if err != nil {
		return problem(err)
	}
	defer store.Close()
	projectKey := cache.ProjectKey(root)
	release, err := store.Lock(ctx, projectKey)
	if err != nil {
		return problem(err)
	}
	defer release()
	session := event.SessionID
	if session == "" {
		session = event.ConversationID
	}
	if session == "" {
		return problem(fmt.Errorf("hook input has no session identifier"))
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(projectKey+"/"+client+"/"+session)))
	previous := state{}
	if data, err := store.Read(key); err == nil {
		if err := json.Unmarshal(data, &previous); err != nil {
			return problem(fmt.Errorf("invalid completion state"))
		}
	} else if !os.IsNotExist(err) {
		return problem(err)
	}
	if !event.Active && event.LoopCount == 0 || time.Since(previous.Updated) > time.Hour {
		previous = state{}
	}
	result := check(ctx)
	var text bytes.Buffer
	if err := textreport.Write(&text, result, textreport.Options{}); err != nil {
		return problem(err)
	}
	message := strings.TrimSpace(text.String())
	if len(message) > 16000 {
		message = string([]rune(message)[:min(8000, len([]rune(message)))]) + "\nReport truncated; run phpwarden check --format json for all findings."
	}
	if result.ExitCode == 130 || ctx.Err() != nil {
		if ctx.Err() != nil {
			return problem(ctx.Err())
		}
		return Decision{}
	}
	blocked := result.ExitCode != 0
	if !blocked {
		if !result.Complete {
			fmt.Fprintln(output, message)
			return Feedback(client, "PHPWarden analysis is incomplete; report this limitation.\n"+message, false)
		}
		previous.Attempts = 0
	} else if previous.Attempts >= settings.MaxAttempts || event.LoopCount >= settings.MaxAttempts {
		message = "PHPWarden stopped automatic retries; unresolved checks remain. Report them to the user.\n" + message
		fmt.Fprintln(output, message)
		return Feedback(client, message, false)
	} else {
		previous.Attempts++
		message = "PHPWarden found blocking checks. Fix issues within the user's task, then finish again. Do not weaken rules, add baselines or disable checks just to pass. Use phpwarden doctor for environment failures.\n" + message
	}
	previous.Updated = time.Now().UTC()
	data, _ := json.Marshal(previous)
	if err := store.Write(key, data); err != nil {
		return problem(err)
	}
	if blocked {
		return Feedback(client, message, true)
	}
	return Decision{}
}
