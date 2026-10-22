package profile

type Facts struct {
	Version       *string           `json:"version"`
	VersionSource *string           `json:"version_source"`
	WebRoot       *string           `json:"web_root"`
	RootSource    *string           `json:"root_source"`
	SourceRoots   []string          `json:"source_roots"`
	Exclusions    []string          `json:"exclusions"`
	Inputs        map[string]string `json:"inputs"`
	Installed     *Installed        `json:"installed"`
	PHP           []PHPRequirement  `json:"php_requirements"`
	PlatformPHP   *string           `json:"platform_php"`
}

type Installed struct {
	Path    string  `json:"path"`
	Status  string  `json:"status"`
	Version *string `json:"version"`
}

type PHPRequirement struct {
	Source     string `json:"source"`
	Constraint string `json:"constraint"`
	Dev        bool   `json:"dev"`
	Conflict   bool   `json:"conflict"`
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
