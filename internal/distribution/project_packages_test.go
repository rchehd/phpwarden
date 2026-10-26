package distribution

import "testing"

func TestComposerPackageVersion(t *testing.T) {
	for input, want := range map[string]string{
		"v1.2.3":             "1.2.3",
		"v1.2.3-rc.1":        "1.2.3-rc.1",
		"v1.2.3-alpha.1":     "1.2.3-alpha.1",
		"v1.2.3-beta2":       "1.2.3-beta2",
		"v1.2.3-dev":         "1.2.3-dev",
		"v0.0.0-dev.a38dff3": "dev-0.0.0-dev.a38dff3",
		"v1.2.3-preview.1":   "dev-1.2.3-preview.1",
		"v1.2.3-rc.1.custom": "dev-1.2.3-rc.1.custom",
	} {
		t.Run(input, func(t *testing.T) {
			if actual := composerPackageVersion(input); actual != want {
				t.Fatalf("got %q, want %q", actual, want)
			}
		})
	}
}
