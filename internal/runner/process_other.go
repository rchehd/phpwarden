//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package runner

import (
	"os/exec"
	"time"
)

func prepareProcess(command *exec.Cmd) { command.WaitDelay = time.Second }
func cleanupProcess(*exec.Cmd)         {}
