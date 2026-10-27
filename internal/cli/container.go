package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/worker"
)

func containerCLI(ctx context.Context, arguments []string, cwd string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(arguments) == 0 || arguments[0] == "--help" {
		fmt.Fprintln(stdout, "Usage: phpwarden container <init|check|fix|config|scope|resolve|hook pre-commit> [options]\nUse the Compose host wrapper for isolated network auditing and host hooks.")
		return 0
	}
	if arguments[0] == "audit-export" {
		return exportContainerAudit(arguments[1:], cwd, stderr)
	}
	if arguments[0] == "audit-cache" {
		if len(arguments) != 1 {
			fmt.Fprintln(stderr, "container audit-cache accepts no options")
			return 2
		}
		return cacheContainerAudit(ctx, cwd, stderr)
	}
	if !slices.Contains([]string{"init", "check", "fix", "config", "scope", "resolve", "--version", "--build-info", "--licenses", "hook"}, arguments[0]) || arguments[0] == "hook" && (len(arguments) < 2 || arguments[1] != "pre-commit") {
		fmt.Fprintln(stderr, "This command requires the host CLI; Compose supports init, check, fix, config, scope, resolve and hook pre-commit.")
		return 2
	}
	if arguments[0] == "init" {
		args, err := parse(arguments)
		if err != nil {
			return fail(stdout, stderr, args, "usage", err)
		}
		if args.hooks != "" && args.hooks != "none" {
			return fail(stdout, stderr, args, "usage", fmt.Errorf("completion hooks require a host CLI installation; use --hooks none with Compose"))
		}
		if args.preCommit && os.Getenv("PHPWARDEN_EXECUTABLE") == "" {
			return fail(stdout, stderr, args, "usage", fmt.Errorf("install pre-commit through the Compose host wrapper"))
		}
		if args.hooks == "" {
			arguments = append(slices.Clone(arguments), "--hooks", "none")
		}
	}
	if arguments[0] == "check" || arguments[0] == "fix" {
		fix := arguments[0] == "fix"
		args, err := parse(arguments)
		if err != nil {
			return fail(stdout, stderr, args, "usage", err)
		}
		arguments = slices.DeleteFunc(slices.Clone(arguments), func(s string) bool { return s == "--no-cache" })
		arguments = append([]string{"worker", "check"}, arguments[1:]...)
		if fix {
			arguments = append(arguments, "--fix")
		}
	}
	return runContext(worker.WithCachedAudit(ctx), arguments, cwd, stdin, stdout, stderr, true)
}

func exportContainerAudit(arguments []string, cwd string, stderr io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(stderr, "Cannot prepare isolated audit:", err); return 2 }
	args, err := parse(arguments)
	if err != nil {
		return fail(err)
	}
	only, err := checker.ParseOnly(args.only)
	if err != nil {
		return fail(err)
	}
	location, err := config.Resolve(config.LocationOptions{CWD: cwd, Project: args.project, Config: args.config})
	if err != nil {
		return fail(err)
	}
	effective, err := config.Load(location)
	if err != nil {
		return fail(err)
	}
	settings := effective.Config.Tools.ComposerAudit
	settings.Enabled = checker.Enabled("composer-audit", settings.Enabled, only)
	if len(args.command) > 0 && args.command[0] == "fix" || args.fix {
		settings.Enabled = false
	}
	directory := os.Getenv("PHPWARDEN_AUDIT_EXPORT")
	if directory == "" {
		return fail(fmt.Errorf("PHPWARDEN_AUDIT_EXPORT is required"))
	}
	if settings.Enabled {
		for _, name := range []string{"composer.json", "composer.lock"} {
			input, err := os.ReadFile(filepath.Join(config.ComposerDir(location.Root, effective.Config), name))
			if os.IsNotExist(err) && name == "composer.lock" {
				settings.Enabled = false
				break
			}
			if err != nil {
				return fail(err)
			}
			if len(input) > 32<<20 {
				return fail(fmt.Errorf("Composer metadata exceeds size limit"))
			}
			if err := os.WriteFile(filepath.Join(directory, name), input, 0o600); err != nil {
				return fail(err)
			}
		}
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(directory, "audit.json"), data, 0o600); err != nil {
		return fail(err)
	}
	return 0
}

func cacheContainerAudit(ctx context.Context, cwd string, stderr io.Writer) int {
	data, err := os.ReadFile(filepath.Join(cwd, "audit.json"))
	var settings config.ComposerAudit
	if err != nil || json.Unmarshal(data, &settings) != nil {
		fmt.Fprintln(stderr, "Cannot read isolated audit settings")
		return 2
	}
	if !settings.Enabled {
		return 0
	}
	cache, err := composeraudit.CacheDir()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	result := composeraudit.Run(ctx, cwd, settings, composeraudit.NativeFetcher(composeraudit.Bundle(), runner.Exec{}), cache, time.Now())
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintln(stderr, diagnostic.Message)
	}
	if result.Check.Status == "error" || result.Check.Status == "unavailable" {
		return 3
	}
	return 0
}
