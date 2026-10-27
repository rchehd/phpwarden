package perfruntime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func readOptional(name string) ([]byte, error) {
	if name == "" {
		return nil, nil
	}
	data, err := readRegular(name, 4<<20)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func safeParents(name string) error {
	for parent := filepath.Dir(name); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) || err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("client config parents must be real directories")
		}
		if parent == filepath.Dir(parent) {
			return nil
		}
	}
}

func writeClientEdit(edit clientEdit) error {
	if err := safeParents(edit.path); err != nil {
		return err
	}
	current, err := readOptional(edit.path)
	if err != nil || !bytes.Equal(current, edit.before) || (current == nil) != (edit.before == nil) {
		return fmt.Errorf("client configuration changed concurrently")
	}
	if edit.remove {
		if current == nil {
			return nil
		}
		return os.Remove(edit.path)
	}
	if err := os.MkdirAll(filepath.Dir(edit.path), 0700); err != nil {
		return err
	}
	return atomic(edit.path, edit.after)
}

func applyClientEdits(ctx context.Context, edits []clientEdit) error {
	applied := []clientEdit{}
	for _, edit := range edits {
		err := ctx.Err()
		if err == nil {
			err = writeClientEdit(edit)
		}
		if err != nil {
			rollbackFailed := false
			for i := len(applied) - 1; i >= 0; i-- {
				if rollbackFailed {
					break
				}
				old := applied[i]
				if err := writeClientEdit(clientEdit{path: old.path, before: old.after, after: old.before, remove: old.before == nil}); err != nil {
					rollbackFailed = true
				}
			}
			if rollbackFailed {
				return fmt.Errorf("client update interrupted by concurrent changes; ownership saved, retry to recover")
			}
			return fmt.Errorf("client update failed; settings restored, ownership saved for retry: %w", err)
		}
		applied = append(applied, edit)
	}
	return nil
}
