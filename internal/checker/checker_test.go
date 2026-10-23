package checker

import "testing"

func TestExplicitCheckerSelection(t *testing.T) {
	for _, value := range []string{"debug", "phpstan", "twig-cs-fixer", "composer-audit,composer-validate", "syntax", "syntax,debug", ""} {
		ids, err := ParseOnly(value)
		if err != nil {
			t.Fatal(err)
		}
		if value == "syntax,debug" && (ids[0] != "debug" || ids[1] != "syntax") {
			t.Fatal("selection is not canonical")
		}
		if value == "syntax" && (!Enabled("syntax", false, ids) || Enabled("debug", true, ids)) {
			t.Fatal("selection did not override defaults")
		}
	}
	for _, value := range []string{"phpmd", "syntax,", ",debug", "debug,debug", "debug, syntax"} {
		if _, err := ParseOnly(value); err == nil {
			t.Fatalf("invalid selection accepted: %q", value)
		}
	}
}
