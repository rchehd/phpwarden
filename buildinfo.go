package phpwarden

import (
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
)

var Version = "dev"
var Commit = "unknown"
var Distribution = "source"
var ManifestBase64 string

func ReleaseManifest() ([]byte, error) {
	if ManifestBase64 == "" {
		return nil, nil
	}
	if len(ManifestBase64) > 2<<20 {
		return nil, fmt.Errorf("embedded release manifest exceeds limit")
	}
	data, err := base64.StdEncoding.DecodeString(ManifestBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid embedded release manifest encoding")
	}
	return data, nil
}

type Info struct {
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	Distribution   string `json:"distribution"`
	Platform       string `json:"platform"`
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
}

func Metadata(platform string) (Info, error) {
	result := Info{Version: Version, Commit: Commit, Distribution: Distribution, Platform: platform}
	manifest, err := ReleaseManifest()
	if err != nil {
		return result, err
	}
	if manifest != nil {
		result.ManifestSHA256 = fmt.Sprintf("%x", sha256.Sum256(manifest))
	}
	return result, nil
}

//go:embed docker-bake.json
var buildDefinition []byte

var developmentToolset = sync.OnceValue(func() string {
	var definition struct {
		Variables map[string]struct {
			Default string `json:"default"`
		} `json:"variable"`
	}
	if err := json.Unmarshal(buildDefinition, &definition); err != nil {
		panic("invalid bundled Docker Bake definition: " + err.Error())
	}
	value := definition.Variables["PHPWARDEN_TOOLSET"].Default
	if !regexp.MustCompile(`^static-dev-v[1-9][0-9]*$`).MatchString(value) {
		panic("bundled Docker Bake definition has an invalid PHPWARDEN_TOOLSET")
	}
	return value
})

// DevelopmentToolset returns the shared CLI and image compatibility revision.
func DevelopmentToolset() string { return developmentToolset() }
