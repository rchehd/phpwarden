package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const repository = "rchehd/phpwarden"
const apiRoot = "https://api.github.com/repos/" + repository + "/releases/"
const downloadRoot = "https://github.com/" + repository + "/releases/download/"

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type Client struct{ HTTP *http.Client }

func NewClient() Client {
	return Client{HTTP: &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		host := request.URL.Hostname()
		if len(via) > 5 || request.URL.Scheme != "https" || request.URL.User != nil || request.URL.Port() != "" || host != "github.com" && host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com" && host != "api.github.com" {
			return fmt.Errorf("unexpected release download redirect")
		}
		return nil
	}}}
}

func (client Client) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "PHPWarden updater")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("release request failed; check network access and retry")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case 404:
			return nil, fmt.Errorf("no published release or matching asset exists; Actions artifacts are a separate development channel")
		case 403, 429:
			return nil, fmt.Errorf("GitHub release API access was denied or rate limited; retry later")
		}
		return nil, fmt.Errorf("release request returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return nil, fmt.Errorf("release response exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("release download was interrupted")
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("release response exceeds size limit")
	}
	return data, nil
}

func (client Client) Release(ctx context.Context, to string) (Release, error) {
	path := "latest"
	if to != "" {
		path = "tags/" + url.PathEscape(to)
	}
	data, err := client.get(ctx, apiRoot+path, 2<<20)
	if err != nil {
		return Release{}, err
	}
	var release Release
	if json.Unmarshal(data, &release) != nil {
		return release, fmt.Errorf("invalid GitHub release response")
	}
	normalized, err := Version(release.Tag)
	if err != nil || normalized != release.Tag || release.Draft || to == "" && (release.Prerelease || strings.Contains(normalized, "-")) || to != "" && release.Tag != to {
		return release, fmt.Errorf("GitHub returned an unexpected release identity")
	}
	seen := map[string]bool{}
	for _, asset := range release.Assets {
		if seen[asset.Name] {
			return release, fmt.Errorf("duplicate release asset")
		}
		seen[asset.Name] = true
	}
	return release, nil
}

func (release Release) asset(name string) (Asset, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			if asset.URL != downloadRoot+release.Tag+"/"+name || asset.Size <= 0 {
				return Asset{}, fmt.Errorf("invalid release asset URL or size")
			}
			return asset, nil
		}
	}
	return Asset{}, fmt.Errorf("release %s has no %s asset for this platform", release.Tag, name)
}

func (client Client) Download(ctx context.Context, asset Asset, limit int64) ([]byte, error) {
	if !strings.HasPrefix(asset.URL, downloadRoot) || asset.Size > limit {
		return nil, fmt.Errorf("invalid or oversized release asset")
	}
	data, err := client.get(ctx, asset.URL, limit)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != asset.Size {
		return nil, fmt.Errorf("release asset size mismatch")
	}
	return data, nil
}
