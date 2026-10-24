package cache

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStoreConcurrentAtomicWrites(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cache")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := strings.Repeat("a", 64)
	var wait sync.WaitGroup
	for i := range 8 {
		wait.Go(func() {
			release, err := store.Lock(context.Background(), key)
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			data := []byte(fmt.Sprintf(`{"writer":%d,"data":"%s"}`, i, strings.Repeat("x", 4096)))
			if err := store.Write(key, data); err != nil {
				t.Error(err)
				return
			}
			got, err := store.Read(key)
			if err != nil || string(got) != string(data) {
				t.Errorf("non-atomic cache read: %v", err)
			}
		})
	}
	wait.Wait()
	entries, _ := os.ReadDir(directory)
	if len(entries) != 2 {
		t.Fatalf("temporary files leaked: %v", entries)
	}
}

func TestCancelledLockAndUnsafeEntries(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cache")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := strings.Repeat("b", 64)
	release, err := store.Lock(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := store.Lock(ctx, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock ignored cancellation: %v", err)
	}
	release()
	unlock, err := store.Lock(context.Background(), key)
	if err != nil {
		t.Fatal("cancelled waiter kept lock:", err)
	}
	unlock()
	outside := filepath.Join(t.TempDir(), "data")
	os.WriteFile(outside, []byte("original"), 0600)
	os.Symlink(outside, filepath.Join(directory, key+".json"))
	if _, err := store.Read(key); err == nil {
		t.Fatal("followed cache symlink")
	}
	if err := store.Write(key, []byte("cached")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "original" {
		t.Fatal("overwrote symlink target")
	}
	os.Remove(filepath.Join(directory, key+".lock"))
	os.Symlink(outside, filepath.Join(directory, key+".lock"))
	if _, err := store.Lock(context.Background(), key); err == nil {
		t.Fatal("followed lock symlink")
	}
	if err := store.Write("../outside", nil); err == nil {
		t.Fatal("accepted invalid cache key")
	}
}

func TestProjectDirectoryLockUsesNoProjectFiles(t *testing.T) {
	root := t.TempDir()
	release, err := LockProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := LockProject(ctx, root); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("concurrent fixer did not wait", err)
	}
	release()
	unlock, err := LockProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("project lock created source artifacts")
	}
}

func TestLockReleasedWhenOwnerProcessIsKilled(t *testing.T) {
	if directory := os.Getenv("PHPWARDEN_LOCK_HELPER"); directory != "" {
		store, err := Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		release, err := store.Lock(context.Background(), strings.Repeat("c", 64))
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		fmt.Println("locked")
		time.Sleep(time.Minute)
		return
	}
	directory := filepath.Join(t.TempDir(), "cache")
	command := exec.Command(os.Args[0], "-test.run=^TestLockReleasedWhenOwnerProcessIsKilled$")
	command.Env = append(os.Environ(), "PHPWARDEN_LOCK_HELPER="+directory)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	if line, err := bufio.NewReader(output).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("helper failed: %q %v", line, err)
	}
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := store.Lock(ctx, strings.Repeat("c", 64)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("cross-process lock did not wait", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	command.Wait()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := store.Lock(ctx, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal("dead owner retained lock", err)
	}
	release()
}
