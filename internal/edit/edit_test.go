package edit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestApplyPreservesPermissionsAndRejectsConcurrentEdits(t *testing.T) {
	root := t.TempDir()
	name := "source.php"
	filename := filepath.Join(root, name)
	if err := os.WriteFile(filename, []byte("before"), 0o750); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]string{name: Hash([]byte("before"))}
	changes := []Edit{{Path: name, Before: allowed[name], After: []byte("after")}}
	changed, err := Apply(root, changes, allowed)
	if err != nil || !slices.Equal(changed, []string{name}) {
		t.Fatalf("apply: %v, %v", changed, err)
	}
	info, _ := os.Stat(filename)
	content, _ := os.ReadFile(filename)
	if string(content) != "after" || info.Mode().Perm() != 0o750 {
		t.Fatal("replacement changed permissions or contents")
	}
	if changed, err := Apply(root, changes, allowed); err == nil || len(changed) != 0 {
		t.Fatal("stale replacement was accepted")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("temporary replacement files leaked")
	}
}

func TestAllEditsAreValidatedBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.php"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]string{"first.php": Hash([]byte("original")), "missing.php": Hash(nil)}
	changes := []Edit{{Path: "first.php", Before: allowed["first.php"], After: []byte("new")}, {Path: "missing.php", Before: allowed["missing.php"], After: []byte("new")}}
	if _, err := Apply(root, changes, allowed); err == nil {
		t.Fatal("missing input was accepted")
	}
	content, _ := os.ReadFile(filepath.Join(root, "first.php"))
	if string(content) != "original" {
		t.Fatal("earlier edit was written before all inputs were validated")
	}
	changes = []Edit{{Path: "../outside.php", Before: "x", After: []byte("new")}}
	if _, err := Apply(root, changes, allowed); err == nil {
		t.Fatal("foreign path was accepted")
	}
	if err := os.Symlink("first.php", filepath.Join(root, "link.php")); err != nil {
		t.Fatal(err)
	}
	allowed["link.php"] = allowed["first.php"]
	if _, err := Apply(root, []Edit{{Path: "link.php", Before: allowed["link.php"], After: []byte("new")}}, allowed); err == nil {
		t.Fatal("symlink was accepted")
	}
}

func TestValidateRejectsMalformedEdits(t *testing.T) {
	allowed := map[string]string{"demo.php": Hash([]byte("before"))}
	valid := Edit{Path: "demo.php", Before: allowed["demo.php"], After: []byte("after")}
	for _, changes := range [][]Edit{
		{valid, valid},
		{{Path: valid.Path, Before: "stale", After: valid.After}},
		{{Path: valid.Path, Before: valid.Before}},
		{{Path: "other.php", Before: valid.Before, After: valid.After}},
		{{Path: "./demo.php", Before: valid.Before, After: valid.After}},
	} {
		if err := Validate(changes, allowed); err == nil {
			t.Fatalf("malformed edits accepted: %+v", changes)
		}
	}
	if err := Validate([]Edit{{Path: valid.Path, Before: valid.Before, After: []byte{}}}, allowed); err != nil {
		t.Fatalf("empty file replacement rejected: %v", err)
	}
}
