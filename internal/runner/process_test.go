package runner

import (
	"context"
	"testing"
	"time"
)

func TestTimeoutAndMissingExecutable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result := (Exec{}).Run(ctx, Command{Name: "php", Args: []string{"-r", "usleep(5000000);"}})
	if !result.TimedOut || result.Err == nil {
		t.Fatalf("timeout not surfaced: %+v", result)
	}
	result = (Exec{}).Run(context.Background(), Command{Name: "phpwarden-executable-that-does-not-exist"})
	if !result.Unavailable || result.Err == nil {
		t.Fatalf("missing executable not surfaced: %+v", result)
	}
}
