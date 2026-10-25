package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/agenthook"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

type completionProcess struct{ runner.Process }

func (process completionProcess) Run(ctx context.Context, command runner.Command) runner.Result {
	if command.Env == nil {
		command.Env = os.Environ()
	}
	command.Env = append(append([]string{}, command.Env...), "PHPWARDEN_COMPLETION_ACTIVE=1")
	if command.Name == "docker" && len(command.Args) > 0 && command.Args[0] == "run" {
		command.Args = append([]string{"run", "--env", "PHPWARDEN_COMPLETION_ACTIVE=1"}, command.Args[1:]...)
	}
	return process.Process.Run(ctx, command)
}

func completionHook(ctx context.Context, arguments []string, cwd string, stdin io.Reader, stdout, stderr io.Writer) int {
	client, project := "", ""
	seen := map[string]bool{}
	for i := 0; i < len(arguments); i++ {
		name, value, inline := strings.Cut(arguments[i], "=")
		if seen[name] || name != "--client" && name != "--project" && name != "--managed" {
			fmt.Fprintln(stderr, "hook stop accepts --client codex|cursor|claude and --project PATH")
			fmt.Fprintln(stdout, "{}")
			return 2
		}
		seen[name] = true
		if name == "--managed" && !inline {
			continue
		}
		if name == "--managed" || !inline && (i+1 >= len(arguments) || strings.HasPrefix(arguments[i+1], "--")) || inline && value == "" {
			fmt.Fprintln(stderr, "invalid completion hook option value")
			fmt.Fprintln(stdout, "{}")
			return 2
		}
		if !inline && i+1 < len(arguments) {
			i++
			value = arguments[i]
		}
		if name == "--client" {
			client = value
		} else if name == "--project" {
			project = value
		}
	}
	if client != "codex" && client != "cursor" && client != "claude" {
		fmt.Fprintln(stderr, "hook stop requires --client codex|cursor|claude")
		fmt.Fprintln(stdout, "{}")
		return 2
	}
	if project != "" && !filepath.IsAbs(project) {
		project = filepath.Join(cwd, project)
	}
	event, err := agenthook.Read(stdin, client, cwd, project)
	if err != nil {
		fmt.Fprintln(stderr, "PHPWarden completion input:", err)
		_ = writeJSON(stdout, agenthook.Feedback(client, "PHPWarden completion input is invalid; checks were not run.", false))
		return 0
	}
	if event.Status == "aborted" || event.Status == "error" || os.Getenv("PHPWARDEN_COMPLETION_ACTIVE") == "1" {
		fmt.Fprintln(stdout, "{}")
		return 0
	}
	location, err := config.Resolve(config.LocationOptions{CWD: event.CWD, Project: project})
	var effective config.Effective
	if err == nil {
		effective, err = config.Load(location)
	}
	if err != nil {
		message := "PHPWarden could not load project settings: " + err.Error() + ". Run phpwarden doctor and report the incomplete check."
		fmt.Fprintln(stderr, message)
		_ = writeJSON(stdout, agenthook.Feedback(client, message, !event.Active && event.LoopCount == 0))
		return 0
	}
	decision := agenthook.Run(ctx, location.Root, client, event, effective.Config.Agents.Completion, func(ctx context.Context) report.Report {
		fresh, err := config.Load(location)
		if err == nil {
			fresh, err = discovery.Apply(location, fresh)
		}
		if err != nil {
			return report.InputError("config", err)
		}
		selectionOptions, err := scopeOptions(options{command: []string{"check"}}, event.CWD, location, fresh.Config)
		if err != nil {
			return report.InputError("scope", err)
		}
		selection, err := scope.Build(ctx, selectionOptions)
		if err != nil {
			return report.InputError("scope", err)
		}
		if len(selection.Files) == 0 && len(selection.ProjectFiles) == 0 {
			result := report.New()
			result.Scope = &selection
			return result
		}
		return launcher.Check(ctx, location, fresh, selectionOptions, nil, completionProcess{runner.Exec{}})
	}, stderr)
	if err := writeJSON(stdout, decision); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	return 0
}
