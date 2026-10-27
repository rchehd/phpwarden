//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package perfruntime

import (
	"fmt"
	"os"
)

func openInput(name string) (*os.File, error) {
	return nil, fmt.Errorf("safe runtime input handling is unavailable")
}
