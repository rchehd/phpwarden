package worker

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func TestConfiguredCheckerConcurrency(t *testing.T) {
	for _, concurrency := range []int{1, 2, 0, 128} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			location, effective, options := fixture(t)
			effective.Config.Runner.Concurrency = concurrency
			selection, err := scope.Build(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ids := []string{"syntax", "debug", "composer-audit"}
			started := make(chan string, len(ids))
			release := map[string]chan struct{}{}
			for _, id := range ids {
				release[id] = make(chan struct{})
			}
			var active, peak atomic.Int32
			block := func(ctx context.Context, id string) {
				count := active.Add(1)
				defer active.Add(-1)
				for previous := peak.Load(); count > previous; previous = peak.Load() {
					if peak.CompareAndSwap(previous, count) {
						break
					}
				}
				started <- id
				select {
				case <-release[id]:
				case <-ctx.Done():
				}
			}
			process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
				if slices.Contains(command.Args, "-r") {
					block(ctx, "syntax")
					return runner.Result{Stdout: []byte("8.4.26")}
				}
				if slices.Contains(command.Args, "-l") {
					return runner.Result{Stdout: fmt.Appendf(nil, "No syntax errors detected in %s\n", command.Args[len(command.Args)-1])}
				}
				block(ctx, "debug")
				return cleanResult()
			})
			audit := func(ctx context.Context, cfg config.ComposerAudit) checker.Outcome {
				block(ctx, "composer-audit")
				return checker.Outcome{Check: report.Check{ID: "composer-audit", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn},
					Findings: []report.Finding{{Checker: "composer-audit", Rule: "test.warning", Kind: "global", Severity: "warning", Message: "Retained audit warning"}}}
			}
			var stages []string
			ctx = progress.WithReporter(ctx, func(stage string) { stages = append(stages, stage) })
			done := make(chan report.Report, 1)
			go func() { done <- analyze(ctx, location, effective.Config, nil, selection, ids, audit, process) }()
			want := concurrency
			if want == 0 || want > len(ids) {
				want = len(ids)
			}
			var running []string
			for range want {
				select {
				case id := <-started:
					running = append(running, id)
				case <-ctx.Done():
					t.Fatal("configured analyzers did not run concurrently")
				}
			}
			for _, id := range running {
				close(release[id])
			}
			for count := want; count < len(ids); count++ {
				select {
				case id := <-started:
					close(release[id])
				case <-ctx.Done():
					t.Fatal("queued analyzer did not run")
				}
			}
			select {
			case result := <-done:
				if result.ExitCode != 0 || !result.Complete || len(result.Checks) != len(ids) || len(result.Findings) != 1 || len(stages) == 0 {
					t.Fatalf("concurrent result lost outcomes: %+v", result)
				}
				if peak.Load() != int32(want) || active.Load() != 0 {
					t.Fatalf("concurrency peak=%d active=%d, want %d and 0", peak.Load(), active.Load(), want)
				}
			case <-ctx.Done():
				t.Fatal("analysis did not finish")
			}
		})
	}
}

func TestParallelCancellationPreservesIncompleteResult(t *testing.T) {
	location, effective, options := fixture(t)
	effective.Config.Runner.Concurrency = 0
	selection, err := scope.Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	process := processFunc(func(ctx context.Context, _ runner.Command) runner.Result {
		started <- struct{}{}
		<-ctx.Done()
		return runner.Result{Err: ctx.Err()}
	})
	done := make(chan report.Report, 1)
	go func() {
		done <- analyze(ctx, location, effective.Config, nil, selection, []string{"debug", "syntax"}, nil, process)
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("analyzers did not start")
		}
	}
	cancel()
	select {
	case result := <-done:
		if result.Complete || result.ExitCode != 3 || len(result.Checks) != 2 {
			t.Fatalf("cancelled analysis accepted: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled analyzers did not stop")
	}
}
