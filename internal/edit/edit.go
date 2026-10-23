package edit

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

const MaxBytes = 32 << 20

type Edit struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  []byte `json:"after"`
}

func Hash(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func Validate(edits []Edit, allowed map[string]string) error {
	seen := map[string]bool{}
	total := 0
	for _, change := range edits {
		total += len(change.After)
		if err := projectpath.Validate(change.Path); err != nil {
			return err
		}
		if path.Clean(change.Path) != change.Path || seen[change.Path] || allowed[change.Path] == "" ||
			change.Before != allowed[change.Path] || change.After == nil || total > MaxBytes {
			return fmt.Errorf("fix edits contain an unexpected path, stale hash, duplicate file or oversized content")
		}
		seen[change.Path] = true
	}
	return nil
}

func Read(root *os.Root, name string) ([]byte, fs.FileMode, error) {
	if err := projectpath.Validate(name); err != nil {
		return nil, 0, err
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, 0, err
		}
		if info.Mode()&os.ModeSymlink != 0 || i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("fix inputs must be regular files without symlinks")
		}
	}
	info, err := root.Stat(name)
	if err != nil {
		return nil, 0, err
	}
	data, err := root.ReadFile(name)
	return data, info.Mode().Perm(), err
}

// Each replacement is atomic; a later filesystem error reports every file already written.
func Apply(directory string, edits []Edit, allowed map[string]string) ([]string, error) {
	changed := []string{}
	if err := Validate(edits, allowed); err != nil {
		return changed, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return changed, err
	}
	defer root.Close()
	modes := map[string]fs.FileMode{}
	for _, change := range edits {
		data, mode, err := Read(root, change.Path)
		if err != nil || Hash(data) != change.Before {
			return changed, fmt.Errorf("fix input %q changed before applying edits", change.Path)
		}
		modes[change.Path] = mode
	}
	staged := map[string]string{}
	defer func() {
		for _, name := range staged {
			root.Remove(name)
		}
	}()
	for _, change := range edits {
		if Hash(change.After) == change.Before {
			continue
		}
		name := path.Join(path.Dir(change.Path), ".phpwarden-fix-"+rand.Text())
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return changed, err
		}
		staged[change.Path] = name
		_, err = file.Write(change.After)
		if err == nil {
			err = file.Chmod(modes[change.Path])
		}
		if err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return changed, err
		}
		if closeErr != nil {
			return changed, closeErr
		}
	}
	for _, change := range edits {
		if _, exists := staged[change.Path]; !exists {
			continue
		}
		data, mode, err := Read(root, change.Path)
		if err != nil || Hash(data) != change.Before || mode != modes[change.Path] {
			return changed, fmt.Errorf("fix input %q changed while preparing replacements", change.Path)
		}
	}
	for _, change := range edits {
		name, exists := staged[change.Path]
		if !exists {
			continue
		}
		data, mode, err := Read(root, change.Path)
		if err != nil || Hash(data) != change.Before || mode != modes[change.Path] {
			return changed, fmt.Errorf("fix input %q changed before replacement", change.Path)
		}
		if err := root.Rename(name, change.Path); err != nil {
			return changed, err
		}
		delete(staged, change.Path)
		changed = append(changed, change.Path)
	}
	slices.Sort(changed)
	return changed, nil
}
