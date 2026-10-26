package phpwarden

import "embed"

//go:embed examples/compose/phpwarden examples/compose/phpwarden.compose.yaml
var ComposeAssets embed.FS
