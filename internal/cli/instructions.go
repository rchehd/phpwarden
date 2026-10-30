package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/rchehd/phpwarden/internal/agentinstructions"
	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/projectinit"
)

func instructionsCLI(ctx context.Context, arguments []string, cwd string, stdout, stderr io.Writer) int {
	const usage = "Usage: phpwarden instructions remove --client all|codex,cursor,claude [--project PATH] [--format text|json]\nRemoves only unchanged managed instruction blocks; preserves skills, hooks, config and user text.\n"
	format := "text"
	for i, arg := range arguments {
		if arg == "--format=json" || arg == "--format" && i+1 < len(arguments) && arguments[i+1] == "json" {
			format = "json"
		}
	}
	failure := func(err error) int {
		if errors.Is(err, context.Canceled) {
			if format == "json" {
				if writeErr := writeJSON(stdout, map[string]any{"version": 1, "command": "instructions remove", "exit_code": 130, "diagnostics": []string{"interrupted"}}); writeErr != nil {
					return 3
				}
			} else {
				fmt.Fprintln(stderr, "PHPWarden instructions removal interrupted.")
			}
			return 130
		}
		return fail(stdout, stderr, options{format: format, command: []string{"instructions", "remove"}}, "instructions", err)
	}
	if len(arguments) == 1 && (arguments[0] == "--help" || arguments[0] == "-h") {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(arguments) == 0 || arguments[0] != "remove" {
		return failure(fmt.Errorf("%s", strings.TrimSpace(usage)))
	}
	flags := flag.NewFlagSet("instructions remove", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "project root")
	clients := flags.String("client", "", "client selection")
	output := flags.String("format", format, "text or json")
	help := flags.Bool("help", false, "show help")
	seen := map[string]bool{}
	for _, arg := range arguments[1:] {
		if strings.HasPrefix(arg, "--") {
			name, _, _ := strings.Cut(arg, "=")
			if seen[name] {
				return failure(fmt.Errorf("duplicate option %s", name))
			}
			seen[name] = true
		}
	}
	if err := flags.Parse(arguments[1:]); err != nil {
		return failure(err)
	}
	if *help {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if flags.NArg() != 0 || *output != "text" && *output != "json" {
		return failure(fmt.Errorf("instructions remove supports --client, --project and --format text|json"))
	}
	format = *output
	selected, err := agentskills.ParseClients(*clients)
	if err != nil || len(selected) == 0 {
		return failure(fmt.Errorf("--client requires all or a comma-separated selection of codex,cursor,claude"))
	}
	root, err := projectinit.ProjectRoot(ctx, cwd, *project)
	if err != nil {
		return failure(err)
	}
	plan, err := agentinstructions.PrepareRemoval(ctx, root, selected)
	if err != nil {
		return failure(err)
	}
	rows, err := plan.Remove(ctx)
	if err != nil {
		return failure(err)
	}
	if format == "json" {
		if err := writeJSON(stdout, map[string]any{"version": 1, "command": "instructions remove", "exit_code": 0, "instructions": rows}); err != nil {
			fmt.Fprintln(stderr, "Could not write instructions result.")
			return 3
		}
	} else {
		for _, row := range rows {
			status := "unchanged"
			if row.Removed {
				status = "removed managed block"
			} else if row.Changed {
				status = "updated client selection"
			}
			fmt.Fprintf(stdout, "%s: %s\n", status, row.Path)
		}
	}
	return 0
}
