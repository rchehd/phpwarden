package launcher

import (
	"context"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/projectupdates"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func UpdatesFetcher(effective config.Effective, process runner.Process) projectupdates.ComposerFetcher {
	return func(ctx context.Context, manifest, lock []byte) ([]byte, error) {
		selection, err := toolset.Resolve(effective)
		if err != nil {
			return nil, err
		}
		if err := localEndpoint(ctx, process); err != nil {
			return nil, err
		}
		image, err := ensureImage(ctx, selection, effective.Config.Runner.ImagePull, process)
		if err != nil {
			return nil, err
		}
		return dockerComposerQuery(image, process, "updates")(ctx, manifest, lock, false)
	}
}
