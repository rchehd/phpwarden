package schemas

import (
	"bytes"
	_ "embed"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed config-v1.schema.json
var configV1 []byte

//go:embed report-v1.schema.json
var reportV1 []byte

//go:embed manifest-v1.schema.json
var manifestV1 []byte

var compiledConfig = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compile(configV1, "config-v1.schema.json")
})

var compiledReport = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compile(reportV1, "report-v1.schema.json")
})

var compiledManifest = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compile(manifestV1, "manifest-v1.schema.json")
})

func compile(data []byte, filename string) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	resource := "https://github.com/rchehd/phpwarden/schemas/" + filename
	if err := compiler.AddResource(resource, document); err != nil {
		return nil, err
	}
	return compiler.Compile(resource)
}

func ValidateConfig(value any) error {
	schema, err := compiledConfig()
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

func ValidateReport(value any) error {
	schema, err := compiledReport()
	if err != nil {
		return err
	}
	return schema.Validate(value)
}

func ValidateManifest(value any) error {
	schema, err := compiledManifest()
	if err != nil {
		return err
	}
	return schema.Validate(value)
}
