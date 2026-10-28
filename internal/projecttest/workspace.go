package projecttest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

func workspace(ctx context.Context, root, destination, output string) (map[string]string, error) {
	source, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	result := map[string]string{}
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if name == output || name != root && slices.Contains([]string{".git", ".ddev", ".agents", ".codex", ".cursor", ".claude", "node_modules"}, entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := source.Lstat(relative)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			result[relative] = fmt.Sprintf("dir:%o", info.Mode().Perm())
			if destination != "" {
				return os.MkdirAll(target, 0o700)
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := projectpath.Within(root, filepath.ToSlash(relative)); err != nil {
				return err
			}
			link, err := source.Readlink(relative)
			if err != nil {
				return err
			}
			result[relative] = "link:" + link
			if destination == "" {
				return nil
			}
			if filepath.IsAbs(link) {
				resolved, err := filepath.EvalSymlinks(name)
				if err != nil {
					return err
				}
				relativeTarget, err := filepath.Rel(root, resolved)
				if err != nil {
					return err
				}
				if err := projectpath.Validate(filepath.ToSlash(relativeTarget)); err != nil {
					return err
				}
				link = "/workspace/" + filepath.ToSlash(relativeTarget)
			}
			return os.Symlink(link, target)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot isolate special project file %s", relative)
		}
		input, err := source.Open(relative)
		if err != nil {
			return err
		}
		defer input.Close()
		hash := sha256.New()
		writer := io.Writer(hash)
		if destination != "" {
			output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0o600)
			if err != nil {
				return err
			}
			defer output.Close()
			writer = io.MultiWriter(hash, output)
		}
		if _, err := io.Copy(writer, input); err != nil {
			return err
		}
		result[relative] = fmt.Sprintf("%o:%x", info.Mode().Perm(), hash.Sum(nil))
		return nil
	})
	return result, err
}

func artifacts(directory string) (string, error) {
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base := filepath.Join(cache, "phpwarden", "tests")
		if err := os.MkdirAll(base, 0o700); err != nil {
			return "", err
		}
		return os.MkdirTemp(base, "run-")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(directory, "\x00\r\n") {
		return "", fmt.Errorf("invalid output directory")
	}
	if err := os.MkdirAll(filepath.Dir(directory), 0o700); err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(directory))
	if err != nil {
		return "", err
	}
	directory = filepath.Join(parent, filepath.Base(directory))
	if err := os.Mkdir(directory, 0o700); err != nil {
		return "", fmt.Errorf("output directory must be new (existing artifacts are preserved): %w", err)
	}
	return directory, nil
}
