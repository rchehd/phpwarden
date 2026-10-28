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
	Warnings      []string          `json:"warnings"`
}

type Installed struct {
	Path    string  `json:"path"`
	Status  string  `json:"status"`
	Version *string `json:"version"`
	Error   *string `json:"error,omitempty"`
}

type PHPRequirement struct {
	Source     string `json:"source"`
	Constraint string `json:"constraint"`
	Dev        bool   `json:"dev"`
	Conflict   bool   `json:"conflict"`
}

type Defaults struct {
	UnitTestDirectory         string
	PHPUnitConfig             string
	DebugFunctions            []string
	DebugJSFunctions          []string
	DebugTwigFunctions        []string
	JSExtensions              []string
	PHPExtensions             []string
	ModuleConfigDirectories   []string
	ModuleConfigForbiddenKeys []string
	PHPCSStandards            []string
	PHPStanDrupal             bool
	SemgrepEnabled            bool
	EventHorizonEnabled       bool
	DeadCodeEnabled           bool
	RectorEnabled             bool
	PHPMDRulesets             []string
	PHPMDExcludeRules         []string
	TwigExtensions            []string
	TwigCSFixerRemovedRules   []string
	ProjectFiles              []string
}

func Lookup(id string) (Defaults, bool) {
	if id != "drupal" {
		return Defaults{}, false
	}
	return Defaults{
		UnitTestDirectory:         "tests/src/Unit",
		PHPUnitConfig:             "core/phpunit.xml.dist",
		DebugFunctions:            []string{"var_dump", "print_r", "dpm", "dvm", "dsm", "ksm", "kint", "kpr", "dd", "ddm", "ddebug_backtrace", "xdebug_break", "die"},
		DebugJSFunctions:          []string{"console.log", "console.debug", "console.trace", "console.dir"},
		DebugTwigFunctions:        []string{"dump", "kint"},
		JSExtensions:              []string{".js"},
		PHPExtensions:             []string{".php", ".module", ".inc", ".install", ".theme", ".profile", ".engine", ".test"},
		ModuleConfigDirectories:   []string{"config/install", "config/optional", "config/schema"},
		ModuleConfigForbiddenKeys: []string{"uuid", "_core"},
		PHPCSStandards:            []string{"Drupal", "DrupalPractice"},
		PHPStanDrupal:             true,
		SemgrepEnabled:            true,
		EventHorizonEnabled:       true,
		DeadCodeEnabled:           true,
		RectorEnabled:             true,
		PHPMDRulesets:             []string{"unusedcode", "codesize"},
		// Drupal hooks and interface methods have fixed signatures.
		PHPMDExcludeRules: []string{"UnusedFormalParameter"},
		TwigExtensions:    []string{".twig"},
		ProjectFiles:      []string{"composer.json", "composer.lock"},
		// Drupal and SDC templates use the include tag and long-form hashes.
		TwigCSFixerRemovedRules: []string{"TwigCsFixer\\Rules\\Function\\IncludeFunctionRule", "TwigCsFixer\\Rules\\Literal\\CompactHashRule"},
	}, true
}
