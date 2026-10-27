//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package performance

import "os"

func openInput(root *os.Root, name string) (*os.File, error) {
	return nil, problem("unsupported_platform", "Safe report input handling is unavailable on this platform.")
}
