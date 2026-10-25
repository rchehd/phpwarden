package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/projectinit"
)

func initProject(ctx context.Context, args options, cwd string, stdin io.Reader, stdout, stderr io.Writer) int {
	settings := projectinit.Options{CWD: cwd, Project: args.project, ComposerRoot: args.composerRoot, Runtime: args.phpRuntime, Target: args.phpTarget, PreCommit: args.preCommit}
	selected, err := agentskills.ParseClients(args.agents)
	if err != nil {
		return fail(stdout, stderr, args, "usage", err)
	}
	settings.Agents = selected
	settings.Hooks, err = agentskills.ParseClients(args.hooks)
	if err != nil {
		return fail(stdout, stderr, args, "usage", err)
	}
	if interactiveInit(args, stdin, stdout, stderr) {
		reader := bufio.NewReader(stdin)
		settings.SelectChecks = func(choices []projectinit.CheckChoice) ([]string, error) {
			return chooseInitChecks(ctx, reader, stderr, choices)
		}
		if args.agents == "" {
			settings.SelectAgents = func() ([]string, error) {
				return chooseInitAgents(ctx, reader, stderr)
			}
		}
		if args.hooks == "" {
			settings.SelectHooks = func() ([]string, error) {
				return chooseInitIntegrations(ctx, reader, stderr, "Install completion hooks?")
			}
		}
	}
	result, err := projectinit.Run(ctx, settings)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, "PHPWarden init cancelled.")
			return 130
		}
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
		fmt.Fprintf(stdout, "Configuration already exists: %s\n", result.Location.Path)
		writeHookInstallation(stdout, result)
		writeAgentInstallation(stdout, result)
		fmt.Fprintln(stdout, "\nNext: phpwarden check")
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
	writeHookInstallation(stdout, result)
	writeAgentInstallation(stdout, result)
	fmt.Fprintln(stdout, "\nNext: phpwarden check")
	return 0
}

func writeAgentInstallation(stdout io.Writer, result projectinit.Result) {
	if result.AgentSkills == nil {
		return
	}
	verb := "Installed"
	if !result.AgentSkills.Created && !result.AgentSkills.ClaudeCreated {
		verb = "Already installed"
	}
	fmt.Fprintf(stdout, "%s agent skill for %s: %s\n", verb, strings.Join(result.AgentSkills.Clients, ", "), result.AgentSkills.Path)
	if result.AgentSkills.ClaudePath != "" {
		fmt.Fprintf(stdout, "Claude skill link: %s\n", result.AgentSkills.ClaudePath)
	}
	fmt.Fprintln(stdout, "Skills: phpwarden-architecture, phpwarden-review, phpwarden-doctor, phpwarden-upgrade (use / in Cursor/Claude, $ in Codex)")
}

func writeHookInstallation(stdout io.Writer, result projectinit.Result) {
	for _, hook := range result.CompletionHooks {
		verb := "Already installed"
		if hook.Changed {
			verb = "Installed"
		}
		fmt.Fprintf(stdout, "%s %s completion hook: %s\n", verb, hook.Client, hook.Path)
		if hook.Client == "codex" {
			fmt.Fprintln(stdout, "Review and trust the project hook in Codex /hooks before it can run.")
		}
	}
	if result.PreCommit == nil {
		return
	}
	verb := "Installed"
	if !result.PreCommit.Created {
		verb = "Already installed"
	}
	fmt.Fprintf(stdout, "%s pre-commit hook: %s\n", verb, result.PreCommit.Path)
}
