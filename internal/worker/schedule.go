package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/progress"
)

func runCheckers(ctx context.Context, ids []string, concurrency int, run func(string) checker.Outcome) []checker.Outcome {
	outcomes := make([]checker.Outcome, len(ids))
	if concurrency == 1 {
		for i, id := range ids {
			progress.Stage(ctx, "Checking "+id)
			outcomes[i] = run(id)
		}
		return outcomes
	}
	if concurrency == 0 || concurrency > len(ids) {
		concurrency = len(ids)
	}
	jobs := make(chan int, len(ids))
	for i := range ids {
		jobs <- i
	}
	close(jobs)
	var mutex sync.Mutex
	active, completed := 0, 0
	stage := func(running bool) {
		mutex.Lock()
		defer mutex.Unlock()
		if running {
			active++
		} else {
			active--
			completed++
		}
		progress.Stage(ctx, fmt.Sprintf("Running analyzers · %d active · %d/%d complete", active, completed, len(ids)))
	}
	var group sync.WaitGroup
	for range concurrency {
		group.Go(func() {
			for i := range jobs {
				stage(true)
				outcomes[i] = run(ids[i])
				stage(false)
			}
		})
	}
	group.Wait()
	return outcomes
}
