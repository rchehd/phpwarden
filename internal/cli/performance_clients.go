package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/perfruntime"
)

const performanceClientsUsage = `Usage: phpwarden performance connect|disconnect --client CLIENTS [options]

  --client CLIENTS     all or comma-separated codex,cursor,claude
  --project DIR        Exact checkout used by performance setup (default current directory)
  --state-dir DIR      Private state base used by performance setup
  --format text|json   Output format (default text)

connect reads the saved SPX setup and installs a project-local MCP connection.
Codex/Cursor configs must be untracked; existing files must already be ignored.
New configs are locally excluded from Git. Claude uses its private local scope.
Existing servers and settings are preserved. Repeating connect is safe.
disconnect removes only unchanged entries owned by PHPWarden, preserving captures.
Reload the client after connecting; Codex also requires project trust.
`

func performanceClientsCLI(ctx context.Context, args []string, cwd string, stdout, stderr io.Writer) int {
	action := args[0]
	options := perfruntime.ClientOptions{Project: cwd, Executable: os.Getenv("PHPWARDEN_EXECUTABLE")}
	format := "text"
	for i, arg := range args[1:] {
		if arg == "--format=json" || arg == "--format" && i+2 < len(args) && args[i+2] == "json" {
			format = "json"
		}
	}
	invalid := func(message string) int {
		if format == "json" {
			if err := writeJSON(stdout, map[string]any{"command": "performance " + action, "exit_code": 2, "diagnostics": []string{message}}); err != nil {
				fmt.Fprintln(stderr, "Could not write connection result.")
				return 3
			}
		} else {
			fmt.Fprintln(stderr, "phpwarden performance "+action+": "+message)
		}
		return 2
	}
	seen := map[string]bool{}
	for i := 1; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if name == "--help" || name == "-h" {
			fmt.Fprint(stdout, performanceClientsUsage)
			return 0
		}
		if seen[name] {
			return invalid("duplicate connection option")
		}
		seen[name] = true
		switch name {
		case "--client", "--project", "--state-dir", "--format":
		default:
			return invalid("unknown option; see --help")
		}
		if !inline {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return invalid("option requires a value")
			}
			i++
			value = args[i]
		}
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return invalid("option requires a nonempty single-line value")
		}
		switch name {
		case "--client":
			selected, err := agentskills.ParseClients(value)
			if err != nil || len(selected) == 0 {
				return invalid("choose all or a comma-separated selection of codex,cursor,claude")
			}
			options.Clients = selected
		case "--project":
			options.Project = value
		case "--state-dir":
			options.StateDirectory = value
		case "--format":
			if value != "text" && value != "json" {
				return invalid("output supports text or json")
			}
			format = value
		}
	}
	if len(options.Clients) == 0 {
		return invalid("--client is required")
	}
	for _, value := range []*string{&options.Project, &options.StateDirectory} {
		if *value != "" && !filepath.IsAbs(*value) {
			*value = filepath.Join(cwd, *value)
		}
	}
	if buildinfo.Distribution == "container" {
		return invalid("local MCP connections require the host CLI")
	}
	var connections []perfruntime.ClientConnection
	var err error
	if action == "connect" {
		connections, err = perfruntime.Connect(ctx, options)
	} else {
		connections, err = perfruntime.Disconnect(ctx, options)
	}
	if err != nil {
		return invalid(err.Error())
	}
	if format == "json" {
		if err := writeJSON(stdout, map[string]any{"command": "performance " + action, "exit_code": 0, "connections": connections}); err != nil {
			fmt.Fprintln(stderr, "Could not write connection result.")
			return 3
		}
	} else {
		fmt.Fprintln(stdout, "PHPWarden · performance "+action)
		for _, connection := range connections {
			status := "unchanged"
			if connection.Changed {
				status = "updated"
			}
			fmt.Fprintf(stdout, "  ✓ %s · %s · %s\n", connection.Client, status, connection.Path)
		}
		if action == "connect" {
			fmt.Fprintln(stdout, "\nReload the client to discover the performance tools. Codex requires project trust.")
		}
	}
	return 0
}
