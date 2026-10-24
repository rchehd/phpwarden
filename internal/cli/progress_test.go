package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/progress"
)

type progressRecorder chan string

func (recorder progressRecorder) Write(data []byte) (int, error) {
	recorder <- string(data)
	return len(data), nil
}

func TestProgressRefreshesWhileWorkRunsAndStopsBeforeTheReport(t *testing.T) {
	writes := make(progressRecorder, 32)
	display := newTerminalProgress(writes)
	t.Cleanup(display.Stop)
	display.mu.Lock()
	display.started = time.Now().Add(-61 * time.Second)
	display.mu.Unlock()
	display.Stage("Running analyzers")
	var output strings.Builder
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	frames := 0
	for frames < 2 {
		select {
		case row := <-writes:
			output.WriteString(row)
			if strings.Contains(row, "Running analyzers") {
				frames++
			}
		case <-deadline.C:
			t.Fatal("progress did not refresh while work was running")
		}
	}
	display.Stop()
	display.Stop()
	display.Stage("must not appear after stopping")
	for len(writes) > 0 {
		output.WriteString(<-writes)
	}
	if !strings.HasSuffix(output.String(), "\r\x1b[2K") || strings.Contains(output.String(), "must not appear") || !strings.Contains(output.String(), "Running analyzers · 1:") {
		t.Fatalf("progress did not clean up before the final report: %q", output.String())
	}
}

func TestProgressDoesNotWriteToRedirectedOrMachineOutput(t *testing.T) {
	for _, format := range []string{"text", "json", "compact"} {
		var stdout, stderr bytes.Buffer
		ctx, display := startProgress(context.Background(), format, &stdout, &stderr)
		progress.Stage(ctx, "Running checks")
		display.Stop()
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("%s progress polluted redirected output: %q %q", format, stdout.String(), stderr.String())
		}
	}
}
