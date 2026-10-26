package githook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Installation struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

type Plan struct {
	filename string
	content  []byte
}

func PrepareInstall(ctx context.Context, directory, executable string) (Plan, error) {
	explicitExecutable := executable != ""
	top, err := root(ctx, directory)
	if err != nil {
		return Plan{}, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return Plan{}, err
	}
	if top != directory {
		return Plan{}, fmt.Errorf("install the pre-commit hook at the Git root %q", top)
	}
	filename, err := gitPath(ctx, directory, "hooks/pre-commit")
	if err != nil {
		return Plan{}, err
	}
	parent, err := resolvedPath(filepath.Dir(filename))
	if err != nil {
		return Plan{}, err
	}
	filename = filepath.Join(parent, filepath.Base(filename))
	common, err := git(ctx, directory, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Plan{}, err
	}
	commonDir, err := filepath.EvalSymlinks(strings.TrimSuffix(string(common), "\n"))
	if err != nil {
		return Plan{}, err
	}
	relative, err := filepath.Rel(directory, filename)
	inside := err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if !inside && filename != filepath.Join(commonDir, "hooks", "pre-commit") {
		return Plan{}, fmt.Errorf("core.hooksPath points outside this project; integrate 'phpwarden hook pre-commit' in that hook manually")
	}
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return Plan{}, err
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return Plan{}, err
	}
	quoted := "'" + strings.ReplaceAll(executable, "'", "'\"'\"'") + "'"
	content := []byte("#!/bin/sh\n# PHPWarden pre-commit hook v1\nexec " + quoted + " hook pre-commit\n")
	if relative, err := filepath.Rel(directory, executable); explicitExecutable && err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		quotedRelative := "'" + strings.ReplaceAll(filepath.ToSlash(relative), "'", "'\"'\"'") + "'"
		content = []byte("#!/bin/sh\n# PHPWarden pre-commit hook v1\nproject_root=$(git rev-parse --show-toplevel) || exit 2\nexec \"$project_root\"/" + quotedRelative + " hook pre-commit\n")
	}
	plan := Plan{filename: filename, content: content}
	if _, err := plan.existing(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (plan Plan) existing() (bool, error) {
	info, err := os.Lstat(plan.filename)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("existing pre-commit hook %q is not a regular file; leaving it unchanged", plan.filename)
	}
	data, err := os.ReadFile(plan.filename)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(data, plan.content) {
		return false, fmt.Errorf("existing pre-commit hook %q differs; leaving it unchanged; add 'phpwarden hook pre-commit' to your hook manually", plan.filename)
	}
	return true, nil
}

func (plan Plan) Install() (Installation, error) {
	result := Installation{Path: plan.filename}
	if exists, err := plan.existing(); err != nil {
		return result, err
	} else if exists {
		return result, os.Chmod(plan.filename, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(plan.filename), 0o755); err != nil {
		return result, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(plan.filename), ".phpwarden-hook-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(temporary.Name())
	_, err = temporary.Write(plan.content)
	if err == nil {
		err = temporary.Chmod(0o755)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return result, err
	}
	if err = os.Link(temporary.Name(), plan.filename); errors.Is(err, fs.ErrExist) {
		_, err = plan.existing()
		return result, err
	} else if err != nil {
		return result, err
	}
	result.Created = true
	return result, nil
}

func resolvedPath(filename string) (string, error) {
	missing := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(filename)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) || filename == filepath.Dir(filename) {
			return "", err
		}
		missing = append(missing, filepath.Base(filename))
		filename = filepath.Dir(filename)
	}
}
