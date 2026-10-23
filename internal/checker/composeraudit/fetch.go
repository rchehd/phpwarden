package composeraudit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/runner"
)

func Bundle() string {
	executable, _ := os.Executable()
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "composer"))
}

// NativeFetcher runs the bundled Composer on copies of composer.json and composer.lock, so project code and plugins never run.
func NativeFetcher(bundle string, process runner.Process) Fetcher {
	return func(ctx context.Context, composerJSON, composerLock []byte, noDev bool) ([]byte, error) {
		binary := filepath.Join(bundle, "vendor/composer/composer/bin/composer")
		if _, err := os.Stat(binary); err != nil {
			return nil, fmt.Errorf("bundled Composer is unavailable beside the worker binary; rebuild or reinstall the toolset")
		}
		directory, err := os.MkdirTemp("", "phpwarden-composer-audit-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(directory)
		project := filepath.Join(directory, "project")
		if err := os.Mkdir(project, 0o700); err != nil {
			return nil, err
		}
		for name, data := range map[string][]byte{"composer.json": composerJSON, "composer.lock": composerLock} {
			if err := os.WriteFile(filepath.Join(project, name), data, 0o600); err != nil {
				return nil, err
			}
		}
		args := []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "memory_limit=512M", binary, "audit", "--locked", "--format=json", "--abandoned=report",
			"--no-interaction", "--no-plugins", "--no-scripts", "--no-ansi", "--working-dir=" + project}
		if noDev {
			args = append(args, "--no-dev")
		}
		home := filepath.Join(directory, "home")
		environment := append(os.Environ(), "COMPOSER_HOME="+home, "COMPOSER_CACHE_DIR="+filepath.Join(home, "cache"), "COMPOSER_NO_INTERACTION=1", "COMPOSER_DISABLE_NETWORK=0")
		execution := process.Run(ctx, runner.Command{Name: "php", Directory: project, Args: args, Env: environment})
		if execution.Unavailable {
			return nil, fmt.Errorf("PHP executable is unavailable")
		}
		if execution.TimedOut || ctx.Err() != nil {
			return nil, fmt.Errorf("Composer audit did not complete in time")
		}
		if execution.ExitCode < 0 || execution.ExitCode > 3 {
			return nil, fmt.Errorf("Composer audit exited %d: %s", execution.ExitCode, lastLine(execution.Stderr))
		}
		var probe map[string]json.RawMessage
		if !json.Valid(execution.Stdout) || json.Unmarshal(execution.Stdout, &probe) != nil || probe["advisories"] == nil {
			return nil, fmt.Errorf("Composer audit returned an invalid report: %s", lastLine(execution.Stderr))
		}
		return bytes.TrimSpace(execution.Stdout), nil
	}
}

func lastLine(data []byte) string {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" && !strings.HasPrefix(line, "audit [") {
			return line
		}
	}
	return "no error output"
}
