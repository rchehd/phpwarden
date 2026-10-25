package launcher

import (
	"context"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func DiagnoseImage(ctx context.Context, effective config.Effective, process runner.Process) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	selection, err := toolset.Resolve(effective)
	if err != nil {
		return "", err
	}
	if err := localEndpoint(ctx, process); err != nil {
		return "", err
	}
	return inspect(ctx, selection, process)
}
