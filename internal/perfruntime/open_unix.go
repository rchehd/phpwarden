//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package perfruntime

import (
	"os"
	"syscall"
)

func openInput(name string) (*os.File, error) {
	return os.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
