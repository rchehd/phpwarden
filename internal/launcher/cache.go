package launcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
)

type cachedResult struct {
	Version  int             `json:"version"`
	Key      string          `json:"key"`
	Created  time.Time       `json:"created"`
	Checksum string          `json:"checksum"`
	Report   json.RawMessage `json:"report"`
}

type resultCache struct {
	store   *cache.Store
	release func()
	info    *report.Cache
}

func resultKey(root string, request runrequest.Request) string {
	request.RunID = ""
	data, _ := json.Marshal(struct {
		Version int
		Root    string
		Request runrequest.Request
	}{1, cache.ProjectKey(root), request})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func cacheEligibility(request runrequest.Request) string {
	data, _ := json.Marshal(request.Config.Tools)
	settings := map[string]struct {
		Enabled bool `json:"enabled"`
	}{}
	_ = json.Unmarshal(data, &settings)
	for _, id := range checker.Requested(request.Only) {
		if !checker.Enabled(id, settings[id].Enabled, request.Only) {
			continue
		}
		switch id {
		case "debug", "syntax", "yaml", "whitespace", "drupal-config", "composer-audit":
		default:
			return "live_checker:" + id
		}
	}
	return ""
}

func openResultCache(ctx context.Context, location config.Location, request runrequest.Request, enabled bool) (*resultCache, error) {
	c := &resultCache{}
	if !enabled || !request.Config.Runner.ResultCache {
		return c, nil
	}
	c.info = &report.Cache{Status: "bypass"}
	if reason := cacheEligibility(request); reason != "" {
		c.info.Reason = reason
		return c, nil
	}
	directory, err := cache.Directory("results")
	if err == nil {
		if relative, relErr := filepath.Rel(location.Root, directory); relErr == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			c.info.Reason = "cache_inside_project"
			return c, nil
		}
		c.store, err = cache.Open(directory)
	}
	if err != nil {
		c.info.Reason = "cache_unavailable"
		return c, nil
	}
	c.info.Key = resultKey(location.Root, request)
	c.release, err = c.store.Lock(ctx, c.info.Key)
	if err != nil {
		c.store.Close()
		c.store = nil
		if ctx.Err() != nil {
			return c, ctx.Err()
		}
		c.info.Reason = "cache_unavailable"
		return c, nil
	}
	c.info.Status = "miss"
	return c, nil
}

func (c *resultCache) Close() {
	if c.release != nil {
		c.release()
	}
	if c.store != nil {
		c.store.Close()
	}
}

func cacheable(result report.Report) bool {
	if !result.Complete || result.ExitCode < 0 || result.ExitCode > 1 || len(result.Diagnostics) > 0 || len(result.FixedFiles) > 0 || result.Environment == nil || result.Environment.Cache != nil {
		return false
	}
	for _, check := range result.Checks {
		switch check.Status {
		case "passed", "failed":
		case "skipped":
			if check.Reason == nil || (*check.Reason != "disabled" && *check.Reason != "no_files" && *check.Reason != "delegated") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (c *resultCache) Read(request runrequest.Request, now time.Time) (report.Report, bool) {
	if c.store == nil {
		return report.Report{}, false
	}
	data, err := c.store.Read(c.info.Key)
	if err != nil {
		return report.Report{}, false
	}
	var entry cachedResult
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&entry) != nil {
		return report.Report{}, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return report.Report{}, false
	}
	if entry.Version != 1 || entry.Key != c.info.Key || entry.Created.IsZero() || entry.Created.After(now) || now.Sub(entry.Created) > 24*time.Hour || entry.Checksum != fmt.Sprintf("%x", sha256.Sum256(entry.Report)) {
		return report.Report{}, false
	}
	var result report.Report
	if json.Unmarshal(entry.Report, &result) != nil || !cacheable(result) {
		return report.Report{}, false
	}
	result.RunID = request.RunID
	payload, _ := json.Marshal(result)
	result, err = decodeReport(runner.Result{Stdout: payload, ExitCode: result.ExitCode}, request)
	if err != nil {
		return report.Report{}, false
	}
	c.info.Status = "hit"
	return result, true
}

func (c *resultCache) Write(result report.Report, now time.Time) {
	if c.store == nil || c.info.Status == "hit" || !cacheable(result) {
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return
	}
	entry := cachedResult{Version: 1, Key: c.info.Key, Created: now.UTC(), Checksum: fmt.Sprintf("%x", sha256.Sum256(payload)), Report: payload}
	data, err := json.Marshal(entry)
	if err != nil || c.store.Write(c.info.Key, data) != nil {
		c.info.Reason = "cache_unwritable"
	}
}
