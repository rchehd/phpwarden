package projecttest

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const InfectionVersion = "0.35.8"
const infectionExecutable = "/usr/local/lib/phpwarden/mutation/infection.phar"

type MutationPlan struct {
	Settings     config.Mutation   `json:"settings"`
	Scope        scope.Selection   `json:"scope"`
	Roots        []string          `json:"source_roots"`
	BaseContents map[string][]byte `json:"-"`
}

func buildMutation(ctx context.Context, root string, effective config.Effective, selection scope.Selection, native string) (*MutationPlan, error) {
	if !slices.Contains([]string{"8.3", "8.4"}, effective.Config.PHP.Runtime) {
		return nil, fmt.Errorf("mutation testing currently supports PHP 8.3/8.4; the project's PHP runtime is never substituted")
	}
	defaults, _ := profile.Lookup(effective.Config.Profile)
	selection = selection.Filter(nil, nil, func(name string) bool {
		if !strings.HasSuffix(name, ".php") {
			return false
		}
		for _, directory := range defaults.CoverageExcludeDirectories {
			if slices.Contains(strings.Split(name, "/"), directory) {
				return false
			}
		}
		return true
	})
	if len(selection.Files) == 0 {
		return nil, fmt.Errorf("%w: mutation selection contains no PHP source files", ErrUnavailable)
	}
	plan := &MutationPlan{Settings: effective.Config.Mutation, Scope: selection, Roots: slices.Clone(effective.Config.Scope.Include), BaseContents: map[string][]byte{}}
	if native != "" {
		plan.Settings.Config = native
	}
	if plan.Settings.Config == "auto" {
		plan.Settings.Config = ""
		for _, name := range []string{"infection.json5", "infection.json", "infection.json5.dist", "infection.json.dist"} {
			name = path.Join(effective.Config.ComposerRoot, name)
			if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
				plan.Settings.Config = name
				break
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}
	if plan.Settings.Config != "" {
		if err := regularFile(root, plan.Settings.Config); err != nil {
			return nil, err
		}
	}
	if selection.Mode == "diff" && !selection.WholeFiles {
		if selection.MergeBase == nil {
			return nil, fmt.Errorf("mutation diff requires a resolved base commit")
		}
		for _, name := range selection.Files {
			tree := (runner.Exec{}).Run(ctx, runner.Command{Name: "git", Args: []string{"-C", root, "ls-tree", "-z", *selection.MergeBase, "--", name}})
			if tree.ExitCode != 0 || tree.Err != nil {
				return nil, fmt.Errorf("could not inspect mutation base for %s: %s", name, tree.Stderr)
			}
			if len(tree.Stdout) == 0 {
				plan.BaseContents[name] = []byte{}
				continue
			}
			command := runner.Command{Name: "git", Args: []string{"-C", root, "show", *selection.MergeBase + ":" + name}}
			result := (runner.Exec{}).Run(ctx, command)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if result.ExitCode != 0 || result.Err != nil {
				return nil, fmt.Errorf("could not read mutation base for %s: %s", name, result.Stderr)
			}
			plan.BaseContents[name] = result.Stdout
		}
	}
	return plan, nil
}

func mutationGit(ctx context.Context, checkout string, plan MutationPlan) error {
	if plan.Scope.Mode != "diff" || plan.Scope.WholeFiles {
		return nil
	}
	environment := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			environment = append(environment, value)
		}
	}
	environment = append(environment, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	run := func(args ...string) error {
		args = append([]string{"-C", checkout, "-c", "core.hooksPath=/dev/null", "-c", "commit.gpgSign=false", "-c", "user.name=PHPWarden", "-c", "user.email=phpwarden@localhost"}, args...)
		result := (runner.Exec{}).Run(ctx, runner.Command{Name: "git", Args: args, Env: environment})
		if result.ExitCode != 0 || result.Err != nil {
			return fmt.Errorf("could not prepare private mutation diff: %s", result.Stderr)
		}
		return nil
	}
	if err := run("init", "--quiet"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(checkout, ".git/info/exclude"), []byte("*\n"), 0o600); err != nil {
		return err
	}
	current := map[string][]byte{}
	for _, name := range plan.Scope.Files {
		filename := filepath.Join(checkout, filepath.FromSlash(name))
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		current[name] = data
		if err := os.WriteFile(filename, plan.BaseContents[name], 0o600); err != nil {
			return err
		}
		if err := run("add", "--force", "--", name); err != nil {
			return err
		}
	}
	if err := run("commit", "--quiet", "--no-verify", "-m", "Mutation scope base"); err != nil {
		return err
	}
	for name, data := range current {
		if err := os.WriteFile(filepath.Join(checkout, filepath.FromSlash(name)), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}
