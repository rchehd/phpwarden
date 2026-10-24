package runner

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDescendantsAreCleanedOnCancellationAndExit(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel", "exit", "failure"} {
		t.Run(mode, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "child.pid")
			script := `sleep 30 & echo $! > "$1"; wait`
			if mode == "exit" {
				script = `sleep 30 & echo $! > "$1"; exit 0`
			}
			if mode == "failure" {
				script = `sleep 30 & echo $! > "$1"; exit 7`
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if mode == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			done := make(chan Result, 1)
			started := time.Now()
			go func() { done <- (Exec{}).Run(ctx, Command{Name: "sh", Args: []string{"-c", script, "sh", filename}}) }()
			var data []byte
			for time.Since(started) < 2*time.Second {
				data, _ = os.ReadFile(filename)
				if len(data) > 0 {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal("child did not start", err)
			}
			t.Cleanup(func() {
				p, _ := os.FindProcess(pid)
				if p != nil {
					p.Kill()
				}
			})
			if mode == "cancel" {
				cancel()
			}
			result := <-done
			if mode == "timeout" && !result.TimedOut {
				t.Fatal("timeout not reported")
			}
			if time.Since(started) > 2500*time.Millisecond {
				t.Fatal("descendant kept runner pipes open")
			}
			for range 100 {
				status, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
				if os.IsNotExist(err) || strings.Contains(string(status), ") Z ") {
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
			t.Fatalf("descendant %d survived %s", pid, mode)
		})
	}
}
