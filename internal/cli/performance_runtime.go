package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/perfruntime"
)

const performanceRuntimeUsage = `Usage: phpwarden performance setup|status|teardown [options]

  --project DIR            Checkout root (default current directory)
  --state-dir DIR          Private state base outside the project
  --format text|json       Output format

Setup options:
  --environment TYPE       ddev, compose or docker (DDEV can be detected)
  --php X.Y                PHP minor 8.1–8.4 (DDEV reads its config)
  --service NAME           Explicit Compose PHP service
  --container NAME         Explicit bare Docker container name
  --image REF              Existing local base image (resolved to its ID)
  --compose-file FILE      Original Compose file; repeat in original order
  --env-file FILE          Original Compose env file; repeat in original order
  --compose-project NAME   Original Compose project name
  --profile NAME           Active Compose profile; repeat as needed
  --container-root DIR     Source root inside PHP container (DDEV: /var/www/html)
  --spx-archive FILE        Offline source archive; pinned SHA256 is required
  --http                   Enable keyed HTTP profiling for allowed clients
  --http-allow LIST        IP/CIDR list (default 127.0.0.1,::1)

  --apply                  Setup: build/restart selected runtime; teardown: restore it

Without --apply, setup prepares local adapters and teardown removes only owned files.
status checks the saved runtime binding without starting/stopping containers.
Bare Docker setup --apply builds an image; container recreation remains manual.
Tracked project files and shared .gitignore are never edited. Captures are preserved.
`

func performanceRuntimeCLI(ctx context.Context, args []string, cwd string, stdout, stderr io.Writer) int {
	action := args[0]
	args = args[1:]
	options := perfruntime.Options{Project: cwd}
	format := "text"
	for i, arg := range args {
		if arg == "--format=json" || arg == "--format" && i+1 < len(args) && args[i+1] == "json" {
			format = "json"
		}
	}
	write := func(result perfruntime.Report) int {
		if format == "json" {
			if err := writeJSON(stdout, result); err != nil {
				fmt.Fprintln(stderr, "Could not write performance status.")
				return 3
			}
		} else {
			fmt.Fprintf(stdout, "PHPWarden · performance %s · %s\n", action, result.State)
			if result.Environment != "" {
				fmt.Fprintf(stdout, "  Environment: %s\n", result.Environment)
			}
			if result.Reports != "" {
				fmt.Fprintf(stdout, "  Reports: %s\n", result.Reports)
			}
			if result.HTTPKeyFile != "" {
				fmt.Fprintf(stdout, "  HTTP key file: %s\n", result.HTTPKeyFile)
			}
			for _, message := range result.Diagnostics {
				fmt.Fprintf(stdout, "  ✗ %s\n", message)
			}
			for _, message := range result.NextSteps {
				fmt.Fprintf(stdout, "\n%s\n", message)
			}
			if len(result.ReadCommand) > 0 {
				fmt.Fprintln(stdout, "\nRead captures:")
				parts := make([]string, len(result.ReadCommand))
				for i, arg := range result.ReadCommand {
					parts[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
				}
				fmt.Fprintln(stdout, "  "+strings.Join(parts, " "))
			}
			if len(result.LaunchArguments) > 0 {
				data, _ := json.Marshal(result.LaunchArguments)
				fmt.Fprintf(stdout, "\nBare Docker launch arguments (append original launch options):\n  %s\n", data)
			}
			fmt.Fprintf(stdout, "\nResult: %s · exit %d\n", result.State, result.ExitCode)
		}
		return result.ExitCode
	}
	invalid := func(message string) int { return write(perfruntime.Invalid(action, message)) }
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if name == "--help" || name == "-h" {
			fmt.Fprint(stdout, performanceRuntimeUsage)
			return 0
		}
		repeat := name == "--compose-file" || name == "--env-file" || name == "--profile"
		if seen[name] && !repeat {
			return invalid("Duplicate performance option.")
		}
		seen[name] = true
		if name == "--apply" || name == "--http" {
			if inline || action == "status" || name == "--http" && action != "setup" {
				return invalid("Invalid lifecycle flag for this action.")
			}
			if name == "--apply" {
				options.Apply = true
			} else {
				options.HTTP = true
			}
			continue
		}
		common := name == "--project" || name == "--state-dir" || name == "--format"
		setup := name == "--environment" || name == "--php" || name == "--service" || name == "--container" || name == "--image" || name == "--compose-file" || name == "--env-file" || name == "--compose-project" || name == "--profile" || name == "--container-root" || name == "--spx-archive" || name == "--http-allow"
		if !common && (!setup || action != "setup") {
			return invalid("Unknown lifecycle option; see performance " + action + " --help.")
		}
		if !inline {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return invalid("Lifecycle option requires a value.")
			}
			i++
			value = args[i]
		}
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return invalid("Lifecycle option requires a nonempty single-line value.")
		}
		switch name {
		case "--project":
			options.Project = value
		case "--state-dir":
			options.StateDirectory = value
		case "--environment":
			options.Environment = value
		case "--php":
			options.PHP = value
		case "--service":
			options.Service = value
		case "--container":
			options.Container = value
		case "--image":
			options.Image = value
		case "--compose-file":
			options.ComposeFiles = append(options.ComposeFiles, value)
		case "--env-file":
			options.EnvFiles = append(options.EnvFiles, value)
		case "--compose-project":
			options.ComposeProject = value
		case "--profile":
			options.Profiles = append(options.Profiles, value)
		case "--container-root":
			options.ContainerRoot = value
		case "--spx-archive":
			options.SourceArchive = value
		case "--http-allow":
			options.HTTPAllow = value
		case "--format":
			if value != "json" && value != "text" {
				return invalid("Lifecycle output supports text or json.")
			}
			format = value
		}
	}
	for _, value := range []*string{&options.Project, &options.StateDirectory, &options.SourceArchive} {
		if *value != "" && !filepath.IsAbs(*value) {
			*value = filepath.Join(cwd, *value)
		}
	}
	if buildinfo.Distribution == "container" {
		return invalid("Local performance runtime management requires the host CLI.")
	}
	ctx, display := startProgress(ctx, format, stdout, stderr)
	if display != nil {
		display.Stage("Managing local profiling integration")
		defer display.Stop()
	}
	result := perfruntime.Run(ctx, action, options)
	if display != nil {
		display.Stop()
	}
	return write(result)
}
