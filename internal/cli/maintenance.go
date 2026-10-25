package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/doctor"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/projectupdates"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func projectMaintenance(ctx context.Context, args options, cwd string, stdout, stderr io.Writer) int {
	if args.format == "compact" || args.fix || args.all || args.noCache || args.noInteraction || args.preCommit || args.agents != "" || args.hooks != "" || args.only != "" || args.mode != "" || args.base != "" || args.wholeFiles || args.commits != 0 || args.phpRuntime != "" || args.phpTarget != "" || args.composerRoot != "" {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("%s accepts project/config and text/JSON output; update filters apply only to updates", args.command[0]))
	}
	locationOptions := config.LocationOptions{CWD: cwd, Project: args.project, Config: args.config}
	if args.command[0] == "doctor" {
		if args.security || args.direct || args.noDev || args.offline || args.runtimeVersion != "" {
			return fail(stdout, stderr, args, "usage", fmt.Errorf("update filters require updates"))
		}
		result := doctor.Run(ctx, locationOptions, runner.Exec{})
		if args.format == "json" {
			if err := writeJSON(stdout, result); err != nil {
				fmt.Fprintln(stderr, err)
				return 3
			}
		} else {
			fmt.Fprintln(stdout, "PHPWarden · environment diagnosis")
			for _, item := range result.Items {
				mark := "·"
				if item.Status == "ok" {
					mark = "✓"
				}
				if item.Status == "error" {
					mark = "✗"
				}
				fmt.Fprintf(stdout, "  %s %s: %s\n", mark, item.ID, item.Message)
				if item.Hint != "" {
					fmt.Fprintf(stdout, "    %s\n", item.Hint)
				}
			}
		}
		return result.ExitCode
	}
	location, err := config.Resolve(locationOptions)
	if err != nil {
		return fail(stdout, stderr, args, "config", err)
	}
	effective, err := config.Load(location)
	if err != nil {
		return fail(stdout, stderr, args, "config", err)
	}
	if err := projectupdates.ValidateRuntime(args.runtimeVersion, effective.Config.PHP.Runtime); err != nil {
		return fail(stdout, stderr, args, "usage", err)
	}
	settings := effective.Config.Updates
	settings.Offline = settings.Offline || args.offline
	facts, discoveryErr := discovery.Apply(location, effective)
	if discoveryErr == nil {
		effective = facts
	}
	result := projectupdates.Run(ctx, projectupdates.Request{Location: location, Settings: settings, Branch: effective.Config.PHP.Runtime, ComposerRoot: effective.Config.ComposerRoot, Toolset: toolset.DevelopmentID(effective.Config.PHP.Runtime), Options: projectupdates.Options{Security: args.security, Direct: args.direct, NoDev: args.noDev, RuntimeVersion: args.runtimeVersion}, Composer: launcher.UpdatesFetcher(effective, runner.Exec{}), Now: time.Now()})
	if discoveryErr != nil {
		result.Notes = append(result.Notes, "Installed dependency/layout discovery was not validated: "+discoveryErr.Error())
	} else if err := discovery.Unchanged(location, effective); err != nil {
		result.Complete, result.ExitCode = false, 3
		result.Notes = append(result.Notes, "Project configuration or dependency metadata changed during update assessment; rerun it.")
	}
	if args.format == "json" {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else {
		writeUpdates(stdout, result)
	}
	return result.ExitCode
}

func writeUpdates(output io.Writer, result projectupdates.Report) {
	status := "complete"
	if !result.Complete {
		status = "incomplete"
	}
	fmt.Fprintf(output, "PHPWarden · available updates · %s\n", status)
	current := "unknown"
	if result.PHP.Current != nil {
		current = *result.PHP.Current
	}
	fmt.Fprintf(output, "PHP %s · runtime %s · latest %s · support %s\n", result.PHP.Branch, current, result.PHP.Latest, result.PHP.Support)
	if result.PHP.SecurityRelease {
		fmt.Fprintln(output, "  The latest upstream release is security-tagged; this is not a CVE assessment of the current runtime.")
	}
	fmt.Fprintln(output, "  "+result.PHP.Note)
	for _, item := range result.Packages {
		latest := item.Latest
		if latest == "" {
			latest = "not established"
		}
		label := "update"
		if len(item.Advisories) > 0 {
			label = "SECURITY"
		}
		if item.Abandoned {
			label += " · abandoned"
		}
		fmt.Fprintf(output, "\n%s  %s → %s · %s\n", item.Name, item.Current, latest, label)
		for _, advisory := range item.Advisories {
			fmt.Fprintf(output, "  %s: %s\n  Affected: %s\n", advisory.ID, advisory.Title, advisory.Affected)
			if advisory.Ignored {
				fmt.Fprintln(output, "  Ignored by project Composer policy; shown for upgrade planning.")
			}
			if advisory.Link != "" {
				fmt.Fprintln(output, "  "+advisory.Link)
			}
		}
	}
	if len(result.Packages) == 0 {
		if result.Complete {
			fmt.Fprintln(output, "\nNo package updates or advisories match this selection.")
		} else {
			fmt.Fprintln(output, "\nNo package rows available for this selection; incomplete data cannot establish that packages are up to date.")
		}
	}
	for _, source := range result.Sources {
		fmt.Fprintf(output, "\n%s: %s %s\n", source.ID, source.Status, source.FetchedAt)
		if source.Note != "" {
			fmt.Fprintln(output, "  "+source.Note)
		}
	}
	for _, note := range result.Notes {
		fmt.Fprintln(output, "\n"+note)
	}
}

func workerUpdates(ctx context.Context, arguments []string, cwd string, stdout, stderr io.Writer) int {
	if len(arguments) > 0 {
		fmt.Fprintln(stderr, "worker updates accepts no options")
		return 2
	}
	manifest, err := os.ReadFile(filepath.Join(cwd, "composer.json"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	lock, err := os.ReadFile(filepath.Join(cwd, "composer.lock"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	data, err := projectupdates.NativeComposer(composeraudit.Bundle(), runner.Exec{})(ctx, manifest, lock)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	_, err = fmt.Fprintln(stdout, string(data))
	if err != nil {
		return 3
	}
	return 0
}
