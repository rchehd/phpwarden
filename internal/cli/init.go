package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectinit"
)

func initProject(ctx context.Context, args options, cwd string, stdout, stderr io.Writer) int {
	result, err := projectinit.Run(ctx, projectinit.Options{CWD: cwd, Project: args.project, ComposerRoot: args.composerRoot, Runtime: args.phpRuntime, Target: args.phpTarget})
	if err != nil {
		return fail(stdout, stderr, args, "init", err)
	}
	if args.format == "json" {
		value := response{Version: 1, Command: "init", Valid: true, Location: &result.Location, Init: &result, Diagnostics: []diagnostic{}}
		if err := writeJSON(stdout, value); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
		return 0
	}
	if !result.Created {
		fmt.Fprintf(stdout, "Configuration already exists: %s\n\nNext: phpwarden check\n", result.Location.Path)
		return 0
	}
	fmt.Fprintf(stdout, "Created %s\n\n", result.Location.Path)
	row := func(label string, values ...string) {
		for i, value := range values {
			if i > 0 {
				label = ""
			}
			fmt.Fprintf(stdout, "  %-10s %s\n", label, value)
		}
	}
	facts := result.Project
	project := []string{"Drupal"}
	if facts.Version != nil {
		project[0] += " " + *facts.Version
	}
	if facts.WebRoot != nil {
		project = append(project, "web root "+*facts.WebRoot)
	} else {
		project = append(project, "web root not found")
	}
	if len(facts.SourceRoots) > 0 {
		project = append(project, "custom code: "+strings.Join(facts.SourceRoots, ", "))
	} else {
		project = append(project, "no custom code found; set scope.include")
	}
	row("Project", strings.Join(project, " · "))
	if result.ComposerRoot != "" {
		row("Composer", result.ComposerRoot+"/composer.json")
	}
	row("PHP", fmt.Sprintf("runtime %s (%s) · target %s (%s)", result.Runtime.Version, result.Runtime.Source, result.Target.Version, result.Target.Source))
	row("Image", result.Image)
	if result.Base != nil {
		row("Diff base", *result.Base+" (origin/HEAD); set scope.base to compare against another branch")
	} else {
		row("Diff base", "origin/HEAD is not set; set scope.base or run git remote set-head origin --auto")
	}
	if len(result.NativeConfigs) > 0 {
		uses := []string{}
		for _, native := range result.NativeConfigs {
			uses = append(uses, native.Path+" ("+native.Checker+")")
		}
		row("Uses", strings.Join(uses, ", "))
	}
	if len(facts.Warnings) > 0 {
		row("Warnings", facts.Warnings...)
	}
	fmt.Fprintln(stdout, "\nNext: phpwarden check")
	return 0
}
