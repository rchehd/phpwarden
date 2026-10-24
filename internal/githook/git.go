package githook

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func git(ctx context.Context, directory string, args ...string) ([]byte, error) {
	return gitWithIndex(ctx, directory, "", args...)
}

func gitWithIndex(ctx context.Context, directory, index string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory, "-c", "color.ui=false"}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "GIT_OPTIONAL_LOCKS=0")
	if index != "" {
		command.Env = append(command.Env, "GIT_INDEX_FILE="+index)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func root(ctx context.Context, directory string) (string, error) {
	output, err := git(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("pre-commit requires a Git worktree: %w", err)
	}
	return filepath.EvalSymlinks(strings.TrimSuffix(string(output), "\n"))
}

func gitPath(ctx context.Context, directory, name string) (string, error) {
	output, err := git(ctx, directory, "rev-parse", "--path-format=absolute", "--git-path", name)
	return strings.TrimSuffix(string(output), "\n"), err
}
