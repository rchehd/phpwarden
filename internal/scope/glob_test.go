package scope

import "testing"

func TestGlobBoundaries(t *testing.T) {
	for _, testcase := range []struct {
		pattern  string
		filename string
		expected bool
	}{
		{"**/vendor/**", "vendor/a.php", true},
		{"**/vendor/**", "nested/vendor/a.php", true},
		{"**/vendor/**", "nested/vendor", true},
		{"**/vendor/**", "nested/vendorish/a.php", false},
		{"src/*.php", "src/a.php", true},
		{"src/*.php", "src/nested/a.php", false},
		{"src/**/a?.php", "src/a1.php", true},
		{"src/**/a?.php", "src/nested/deep/a1.php", true},
		{"src/**/a?.php", "src/a12.php", false},
		{"**/.git/**", ".git/config", true},
		{"**/generated/**", "src/Generated/file.php", false},
	} {
		t.Run(testcase.pattern+"/"+testcase.filename, func(t *testing.T) {
			if got := matchGlob(testcase.pattern, testcase.filename); got != testcase.expected {
				t.Fatalf("got %v, expected %v", got, testcase.expected)
			}
		})
	}
}

func TestLineMapMembership(t *testing.T) {
	selection := Selection{LineMap: map[string][]Range{
		"changed.php": {{Start: 3, End: 5}}, "all.php": nil, "deletion.php": {},
	}}
	if !selection.Contains("changed.php", 3) || !selection.Contains("changed.php", 5) ||
		selection.Contains("changed.php", 2) || selection.Contains("changed.php", 6) ||
		!selection.Contains("all.php", 10) || selection.Contains("missing.php", 1) ||
		selection.Contains("all.php", 0) || selection.Contains("deletion.php", 1) {
		t.Fatal("reporting scope must distinguish absent, empty, and whole-file maps")
	}
}
