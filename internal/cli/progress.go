package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/rchehd/phpwarden/internal/progress"
)

type terminalProgress struct {
	writer  io.Writer
	started time.Time
	mu      sync.Mutex
	label   string
	frame   int
	stopped bool
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func startProgress(ctx context.Context, format string, stdout, stderr io.Writer) (context.Context, *terminalProgress) {
	terminal := func(writer io.Writer) bool {
		file, ok := writer.(*os.File)
		if !ok {
			return false
		}
		info, err := file.Stat()
		return err == nil && info.Mode()&os.ModeCharDevice != 0
	}
	if format != "text" || os.Getenv("TERM") == "dumb" || os.Getenv("CI") != "" || !terminal(stdout) || !terminal(stderr) {
		return ctx, nil
	}
	display := newTerminalProgress(stderr)
	return progress.WithReporter(ctx, display.Stage), display
}

func newTerminalProgress(writer io.Writer) *terminalProgress {
	display := &terminalProgress{writer: writer, started: time.Now(), label: "Reading project configuration", stop: make(chan struct{}), done: make(chan struct{})}
	display.Stage(display.label)
	go func() {
		defer close(display.done)
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				display.mu.Lock()
				display.render()
				display.mu.Unlock()
			case <-display.stop:
				return
			}
		}
	}()
	return display
}

func (display *terminalProgress) Stage(label string) {
	if display == nil {
		return
	}
	display.mu.Lock()
	defer display.mu.Unlock()
	display.label = label
	display.render()
}

func (display *terminalProgress) render() {
	if display.stopped {
		return
	}
	elapsed := int(time.Since(display.started).Seconds())
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	fmt.Fprintf(display.writer, "\r\x1b[2K%c PHPWarden · %s · %d:%02d", frames[display.frame%len(frames)], display.label, elapsed/60, elapsed%60)
	display.frame++
}

func (display *terminalProgress) Stop() {
	if display == nil {
		return
	}
	display.once.Do(func() {
		display.mu.Lock()
		display.stopped = true
		display.mu.Unlock()
		close(display.stop)
		<-display.done
		fmt.Fprint(display.writer, "\r\x1b[2K")
	})
}
