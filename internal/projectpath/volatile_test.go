package projectpath

import "testing"

func TestVolatilePaths(t *testing.T) {
	for name, want := range map[string]bool{
		".idea":                                true,
		".ddev":                                true,
		"web/.vscode":                          true,
		"node_modules/.cache":                  true,
		"web/sites/default/files":              true,
		"sites/default/private":                true,
		"web/sites/simpletest":                 true,
		".phpunit.result.cache":                true,
		"web/sites/default":                    false,
		"web/sites/default/settings.php":       false,
		"web/modules/custom/files":             false,
		"web/modules/custom/a/sites/files.php": false,
		"vendor/autoload.php":                  false,
		"phpstan.neon":                         false,
	} {
		if Volatile(name) != want {
			t.Errorf("%s: want %v", name, want)
		}
	}
}
