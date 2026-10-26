package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func imageErrorCode(selection toolset.Selection) string {
	if selection.Development {
		return "image_unavailable"
	}
	return "published_image_unavailable"
}

func ensureImage(ctx context.Context, selection toolset.Selection, policy string, process runner.Process) (string, error) {
	image, err := inspect(ctx, selection, process)
	if err == nil || !errors.Is(err, errImageMissing) || selection.Development {
		return image, err
	}
	if policy == "never" {
		return "", fmt.Errorf("%w; runner.image_pull is never", err)
	}
	if policy != "missing" && policy != "" {
		return "", fmt.Errorf("unsupported image pull policy %q", policy)
	}
	digest, decodeErr := hex.DecodeString(strings.TrimPrefix(selection.Image, "ghcr.io/rchehd/phpwarden@sha256:"))
	if !strings.HasPrefix(selection.Image, "ghcr.io/rchehd/phpwarden@sha256:") || decodeErr != nil || len(digest) != 32 {
		return "", fmt.Errorf("automatic pulls require an official digest-pinned worker image")
	}
	directory, err := cache.Directory("images")
	if err != nil {
		return "", err
	}
	store, err := cache.Open(directory)
	if err != nil {
		return "", fmt.Errorf("cannot lock worker image download: %w", err)
	}
	defer store.Close()
	progress.Stage(ctx, "Waiting for worker image download")
	release, err := store.Lock(ctx, fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Platform+"/"+selection.Image))))
	if err != nil {
		return "", err
	}
	defer release()
	image, err = inspect(ctx, selection, process)
	if err == nil || !errors.Is(err, errImageMissing) {
		return image, err
	}
	progress.Stage(ctx, "Downloading compatible worker image")
	result := process.Run(ctx, runner.Command{Name: "docker", Args: []string{"pull", "--platform", selection.Platform, selection.Image}})
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if result.Err != nil || result.ExitCode != 0 || result.TimedOut || result.Unavailable {
		return "", fmt.Errorf("worker image download failed; check Docker/registry access and retry: %s", strings.TrimSpace(string(result.Stderr)))
	}
	progress.Stage(ctx, "Verifying downloaded worker image")
	return inspect(ctx, selection, process)
}
