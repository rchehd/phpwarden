//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func prepareProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = time.Second
}

func cleanupProcess(command *exec.Cmd) {
	if command.Process != nil {
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
}
