package profile

type Defaults struct {
	DebugFunctions []string
	PHPExtensions  []string
}

func Lookup(id string) (Defaults, bool) {
	if id != "drupal" {
		return Defaults{}, false
	}
	return Defaults{
		DebugFunctions: []string{"var_dump", "print_r", "dpm", "dsm", "kint"},
		PHPExtensions:  []string{".php", ".module", ".inc", ".install", ".theme", ".profile", ".engine", ".test"},
	}, true
}
