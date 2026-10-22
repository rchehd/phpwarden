package scope

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type repository struct {
	root   string
	prefix string
	ctx    context.Context
}

func openRepository(ctx context.Context, root string) (repository, error) {
	repo := repository{root: root, ctx: ctx}
	output, err := repo.git("rev-parse", "--show-toplevel")
	if err != nil {
		return repo, err
	}
	repo.root, err = filepath.EvalSymlinks(strings.TrimSuffix(string(output), "\n"))
	if err != nil {
		return repo, err
	}
	if !utf8.ValidString(repo.root) {
		return repo, fmt.Errorf("Git root is not valid UTF-8")
	}
	prefix, err := filepath.Rel(repo.root, root)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return repo, fmt.Errorf("project root is outside Git worktree")
	}
	if prefix != "." {
		repo.prefix = filepath.ToSlash(prefix) + "/"
	}
	return repo, nil
}

func (repo repository) git(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(repo.ctx, 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo.root, "-c", "color.ui=false"}, args...)...)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			command.Env = append(command.Env, value)
		}
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func (repo repository) pathspec() string {
	if repo.prefix == "" {
		return ":(top,glob)**"
	}
	return ":(top,literal)" + strings.TrimSuffix(repo.prefix, "/")
}

func (repo repository) projectPath(raw []byte) (string, error) {
	if !utf8.Valid(raw) {
		return "", fmt.Errorf("Git filename is not valid UTF-8: %q", raw)
	}
	name := string(raw)
	if !strings.HasPrefix(name, repo.prefix) {
		return "", fmt.Errorf("Git returned a path outside project root: %q", name)
	}
	return strings.TrimPrefix(name, repo.prefix), nil
}

func (repo repository) inventory() (map[string]bool, error) {
	output, err := repo.git("ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", repo.pathspec())
	if err != nil {
		return nil, err
	}
	files := make(map[string]bool)
	for _, raw := range bytes.Split(output, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		name, err := repo.projectPath(raw)
		if err != nil {
			return nil, err
		}
		files[name] = true
	}
	return files, nil
}

func (repo repository) changed(base string) (map[string]bool, error) {
	output, err := repo.git("diff", "--name-status", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", "--ignore-submodules=all", base, "--", repo.pathspec())
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(output, []byte{0})
	changed := make(map[string]bool)
	for i := 0; i+1 < len(parts); i += 2 {
		status, raw := string(parts[i]), parts[i+1]
		if status == "D" {
			continue
		}
		name, err := repo.projectPath(raw)
		if err != nil {
			return nil, err
		}
		changed[name] = status == "A"
	}
	output, err = repo.git("ls-files", "--others", "--exclude-standard", "-z", "--", repo.pathspec())
	if err != nil {
		return nil, err
	}
	for _, raw := range bytes.Split(output, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		name, err := repo.projectPath(raw)
		if err != nil {
			return nil, err
		}
		changed[name] = true
	}
	return changed, nil
}
