package phpwarden

import (
	_ "embed"
	"encoding/json"
)

//go:embed tools/spx/sources.json
var spxSources []byte

type SourcePin struct {
	Commit string `json:"commit"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

func SPXSource() SourcePin {
	var sources struct {
		SPX SourcePin `json:"spx"`
	}
	if err := json.Unmarshal(spxSources, &sources); err != nil {
		panic("invalid bundled SPX source pin")
	}
	return sources.SPX
}
