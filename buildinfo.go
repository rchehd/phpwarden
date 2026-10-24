package phpwarden

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sync"
)

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
