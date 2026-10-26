package toolset

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed publication-v1.json
var publicationMatrix []byte

func PublishedManifest(release string, images map[string]string) ([]byte, error) {
	if release == "" || release == "development" {
		return nil, fmt.Errorf("published images require a distinct release identity")
	}
	var source manifest
	if err := json.Unmarshal(bundledManifest, &source); err != nil {
		return nil, err
	}
	if len(images) != len(source.Entries) {
		return nil, fmt.Errorf("provide one published digest for every bundled PHP runtime")
	}
	var matrix struct {
		Version  int                          `json:"version"`
		Profiles map[string]map[string]string `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(publicationMatrix))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&matrix); err != nil || matrix.Version != 1 {
		return nil, fmt.Errorf("invalid bundled publication matrix")
	}
	source.Release = release
	for index := range source.Entries {
		item := &source.Entries[index]
		image, exists := images[item.PHP.Runtime]
		if !exists || !strings.HasPrefix(image, "ghcr.io/rchehd/phpwarden@sha256:") || !publishedImage.MatchString(image) {
			return nil, fmt.Errorf("PHP %s requires an official repository digest", item.PHP.Runtime)
		}
		constraint := matrix.Profiles[item.Profile][item.PHP.Runtime]
		if constraint == "" {
			return nil, fmt.Errorf("no declared publication range for %s PHP %s", item.Profile, item.PHP.Runtime)
		}
		item.Framework = &constraint
		item.Development = false
		item.Toolset = DevelopmentID(item.PHP.Runtime)
		item.ID = "drupal-static-" + release + "-php" + item.PHP.Runtime
		item.Images = []manifestImage{{Platform: "linux/amd64", Reference: image}}
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, err := ParseManifest(data); err != nil {
		return nil, err
	}
	return data, nil
}
