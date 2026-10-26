package cli

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/selfupdate"
)

func updateCLI(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	settings := selfupdate.Options{}
	format := "text"
	seen := map[string]bool{}
	usageError := func(message string) int { fmt.Fprintln(stderr, message); return 2 }
	for i := 0; i < len(arguments); i++ {
		name, value, inline := strings.Cut(arguments[i], "=")
		if name == "--help" || name == "-h" {
			fmt.Fprint(stdout, "Usage: phpwarden update [--check] [--to VERSION] [--format text|json]\nUpdates the standalone CLI from published GitHub Releases; project settings are preserved.\n")
			return 0
		}
		if seen[name] {
			return usageError("duplicate update option " + name)
		}
		seen[name] = true
		if name == "--check" && !inline {
			settings.Check = true
			continue
		}
		if name != "--to" && name != "--format" {
			return usageError("unknown update option " + name)
		}
		if !inline {
			if i+1 >= len(arguments) || strings.HasPrefix(arguments[i+1], "-") {
				return usageError(name + " requires a value")
			}
			i++
			value = arguments[i]
		}
		if value == "" {
			return usageError(name + " requires a nonempty value")
		}
		if name == "--to" {
			settings.To = value
		} else {
			format = value
		}
	}
	if format != "json" && format != "text" {
		return usageError("update supports text or json")
	}
	metadata, err := buildinfo.Metadata(runtime.GOOS + "/" + runtime.GOARCH)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	settings.Current = metadata
	ctx, display := startProgress(ctx, format, stdout, stderr)
	result := selfupdate.Run(ctx, settings)
	display.Stop()
	if format == "json" {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else {
		fmt.Fprintf(stdout, "PHPWarden · %s", result.Current)
		if result.Target != "" {
			label := "release"
			if result.Available {
				label = "available"
			}
			if result.Updated {
				label = "updated to"
			}
			fmt.Fprintf(stdout, " · %s %s", label, result.Target)
		}
		fmt.Fprintf(stdout, "\n%s\n", result.Message)
	}
	return result.ExitCode
}
