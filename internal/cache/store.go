package cache

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const MaxBytes = 32 << 20

var keyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Store struct{ root *os.Root }

func LockProject(ctx context.Context, directory string) (func(), error) {
	file, err := os.Open(directory)
	if err != nil {
		return nil, err
	}
	if err = lock(ctx, file); err != nil {
		file.Close()
		return nil, err
	}
	return func() { unlock(file); file.Close() }, nil
}

func Directory(name string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "phpwarden", name), nil
}

func Open(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("cache directory must be private and must not be a symlink")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (store *Store) Close() error { return store.root.Close() }

func ProjectKey(root string) string {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(root))))
}

func (store *Store) Read(key string) ([]byte, error) {
	if !keyPattern.MatchString(key) {
		return nil, fmt.Errorf("invalid cache key")
	}
	name := key + ".json"
	info, err := store.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > MaxBytes {
		return nil, fmt.Errorf("unsafe or oversized cache entry")
	}
	file, err := store.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("oversized cache entry")
	}
	return data, err
}

func (store *Store) Write(key string, data []byte) error {
	if !keyPattern.MatchString(key) || len(data) > MaxBytes {
		return fmt.Errorf("invalid cache key or oversized entry")
	}
	name := ".entry-" + rand.Text()
	file, err := store.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer store.root.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return store.root.Rename(name, key+".json")
}

func (store *Store) Lock(ctx context.Context, key string) (func(), error) {
	if !keyPattern.MatchString(key) {
		return nil, fmt.Errorf("invalid cache lock key")
	}
	name := key + ".lock"
	if info, err := store.root.Lstat(name); err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
		return nil, fmt.Errorf("unsafe cache lock")
	}
	file, err := store.root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lock(ctx, file); err != nil {
		file.Close()
		return nil, err
	}
	return func() { unlock(file); file.Close() }, nil
}
