package profile

type Facts struct {
	Version       *string           `json:"version"`
	VersionSource *string           `json:"version_source"`
	WebRoot       *string           `json:"web_root"`
	RootSource    *string           `json:"root_source"`
	SourceRoots   []string          `json:"source_roots"`
	Exclusions    []string          `json:"exclusions"`
	Inputs        map[string]string `json:"inputs"`
}

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
