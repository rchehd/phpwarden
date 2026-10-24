//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package cache

import (
	"context"
	"fmt"
	"os"
)

func lock(context.Context, *os.File) error {
	return fmt.Errorf("cache locks are unsupported on this platform")
}
func unlock(*os.File) {}
