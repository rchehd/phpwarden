//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package cache

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

func lock(ctx context.Context, file *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func unlock(file *os.File) { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
