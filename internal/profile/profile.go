package profile

type Defaults struct {
	DebugFunctions []string
}

func Lookup(id string) (Defaults, bool) {
	if id != "drupal" {
		return Defaults{}, false
	}
	return Defaults{DebugFunctions: []string{"var_dump", "print_r", "dpm", "dsm", "kint"}}, true
}
