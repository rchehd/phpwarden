//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package performance

import (
	"os"
	"syscall"
)

func openInput(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
