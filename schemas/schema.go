package schemas

import (
	"bytes"
	_ "embed"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed config-v1.schema.json
var configV1 []byte

var compiledConfig = sync.OnceValues(func() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(configV1))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	const resource = "https://github.com/rchehd/phpwarden/schemas/config-v1.schema.json"
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, err
	}
	return compiler.Compile(resource)
})

func ValidateConfig(value any) error {
	schema, err := compiledConfig()
	if err != nil {
		return err
	}
	return schema.Validate(value)
}
