package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/runner"
	"go.yaml.in/yaml/v3"
)

type Item struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Report struct {
	Version  int    `json:"version"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Items    []Item `json:"items"`
}

func Run(ctx context.Context, options config.LocationOptions, process runner.Process) Report {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	result := Report{Version: 1, Command: "doctor", Items: []Item{}}
	add := func(id, status, message, hint string) {
		result.Items = append(result.Items, Item{id, status, message, hint})
		if status == "error" {
			result.ExitCode = 3
		}
	}
	location, err := config.Resolve(options)
	if err != nil {
		add("configuration", "error", err.Error(), "Run phpwarden init in the intended project or select --project PATH.")
		return result
	}
	git := process.Run(ctx, runner.Command{Name: "git", Directory: location.Root, Args: []string{"rev-parse", "--show-toplevel"}})
	if git.ExitCode != 0 || git.Err != nil {
		add("git", "error", "The project is not accessible as a Git worktree.", "Check Git availability and the selected project root.")
	} else {
		add("git", "ok", "Git worktree is accessible.", "")
	}
	effective, err := config.Load(location)
	if err != nil {
		add("configuration", "error", err.Error(), "Fix .phpwarden.yaml; phpwarden config validate shows the input error.")
		return result
	}
	add("configuration", "ok", "PHPWarden project configuration is valid.", "")
	facts, err := discovery.Apply(location, effective)
	if err != nil {
		add("discovery", "error", err.Error(), "Check Composer root, framework version and PHP constraints.")
	} else {
		effective = facts
		add("discovery", "ok", "Project layout and Composer metadata were read.", "")
		if installed := facts.Project.Installed; installed != nil && installed.Error != nil {
			add("project_dependencies", "error", *installed.Error, "Restore locked dependencies with composer install in "+config.ComposerDir(location.Root, effective.Config)+" using the project's runtime wrapper.")
		} else if installed == nil || installed.Status != "verified" {
			add("project_dependencies", "warning", "Installed dependency integrity is not fully established.", "Inspect installed metadata before treating dependency-based checks as complete.")
		} else {
			add("project_dependencies", "ok", "Installed Drupal metadata matches the lock file.", "")
		}
		for _, warning := range facts.Project.Warnings {
			add("discovery_note", "warning", warning, "")
		}
	}
	root, rootErr := os.OpenRoot(location.Root)
	if rootErr == nil {
		defer root.Close()
		tools := reflect.ValueOf(effective.Config.Tools)
		for i := range tools.NumField() {
			tool := tools.Field(i)
			field := tool.FieldByName("Config")
			if !field.IsValid() || field.String() == "" || !tool.FieldByName("Enabled").Bool() {
				continue
			}
			id, _, _ := strings.Cut(tools.Type().Field(i).Tag.Get("json"), ",")
			name := field.String()
			info, err := root.Lstat(filepath.FromSlash(name))
			if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 {
				add(id+"_config", "error", fmt.Sprintf("Native config %q is missing, unsafe or too large.", name), "Correct the explicitly configured path or restore the intended native config.")
				continue
			}
			data, err := root.ReadFile(filepath.FromSlash(name))
			if err != nil {
				add(id+"_config", "error", err.Error(), "Check file permissions.")
				continue
			}
			if ext := filepath.Ext(name); ext == ".yaml" || ext == ".yml" || strings.HasSuffix(name, ".yaml.dist") || strings.HasSuffix(name, ".yml.dist") {
				var value yaml.Node
				err = yaml.Unmarshal(data, &value)
			} else if ext == ".json" && !json.Valid(data) {
				err = fmt.Errorf("invalid JSON")
			}
			if err != nil {
				add(id+"_config", "error", name+": "+err.Error(), "Fix native config syntax, then run a targeted check.")
			} else {
				add(id+"_config", "info", name+" is readable; native semantic validation requires its analyzer.", "Run phpwarden check --only "+id+" --mode full.")
			}
		}
	} else {
		add("project_files", "error", rootErr.Error(), "Check project directory permissions.")
	}
	if image, err := launcher.DiagnoseImage(ctx, effective, process); err != nil {
		add("worker_image", "error", err.Error(), "Use a local Docker daemon and a matching published worker image or development build for this CLI.")
	} else {
		add("worker_image", "ok", "Matching worker image available: "+image, "")
	}
	add("application_runtime", "info", "The worker image and php.runtime setting do not establish the deployed PHP patch version.", "For update assessment supply a verified version with updates --runtime-version X.Y.Z.")
	return result
}
