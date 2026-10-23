package drupal

import "testing"

func TestModuleConfigApplicability(t *testing.T) {
	directories := []string{"config/install", "config/optional", "config/schema"}
	for _, tc := range []struct {
		path  string
		roots []string
		want  bool
	}{
		{"web/modules/custom/demo/config/install/demo.yml", nil, true},
		{"modules/demo/config/optional/demo.YAML", nil, true},
		{"docroot/sites/client/modules/custom/demo/config/schema/demo.schema.yml", nil, true},
		{"web/modules/custom/bundle/modules/child/config/install/demo.yml", nil, true},
		{"web/modules/custom/demo/config/install/nested/demo.yml", nil, true},
		{"config/install/demo.yml", []string{"."}, true},
		{"packages/demo/config/install/demo.yml", []string{"packages/demo"}, true},
		{"packages/demo/config/install/demo.yml", []string{"packages"}, false},
		{"packages/demo/config/install/demo.yml", nil, false},
		{"config/install/site.yml", nil, false},
		{"config/sync/site.yml", nil, false},
		{"web/sites/default/files/config/install/site.yml", nil, false},
		{"web/themes/custom/demo/config/install/demo.yml", nil, false},
		{"web/modules/custom/demo/config/export/demo.yml", nil, false},
		{"web/modules/custom/demo/fixtures/config/install/demo.yml", nil, false},
		{"web/modules/custom/demo/config/install/demo.php", nil, false},
		{"web/modules/custom/demo/config/installer/demo.yml", nil, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			if got := ModuleConfigFile(tc.path, directories, tc.roots); got != tc.want {
				t.Fatalf("ModuleConfigFile(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
	if !ModuleConfigFile("modules/demo/defaults/demo.yml", []string{"./defaults/"}, nil) ||
		ModuleConfigFile("modules/demo/config/install/demo.yml", []string{"defaults"}, nil) {
		t.Fatal("directories must replace defaults and match literal normalized paths")
	}
}
