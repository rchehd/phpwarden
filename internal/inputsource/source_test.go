package inputsource

import "testing"

func TestPositionsAndPaths(t *testing.T) {
	data := []byte("я\r\n  😀x")
	for _, item := range []struct {
		offset int64
		want   Position
	}{
		{0, Position{1, 1}}, {2, Position{1, 2}}, {4, Position{2, 1}},
		{10, Position{2, 4}}, {99, Position{2, 5}},
	} {
		if got := At(data, item.offset); got != item.want {
			t.Errorf("offset %d: got %+v, want %+v", item.offset, got, item.want)
		}
	}
	for key, want := range map[string]string{
		"enabled": "$.tools.enabled", "a.b": `$.tools["a.b"]`,
		"a/b~c": `$.tools["a/b~c"]`, "": `$.tools[""]`,
	} {
		if got := Member("$.tools", key); got != want {
			t.Errorf("member %q: %s", key, got)
		}
	}
	if got := At([]byte("a\rb"), 2); got != (Position{2, 1}) {
		t.Fatal(got)
	}
}

func TestUnknownPositionsAreNotInventedAndFileWrappingIsImmutable(t *testing.T) {
	err := &Error{Message: "invalid input"}
	if got := WithFile(err, "composer.json").Error(); got != "composer.json: invalid input" {
		t.Fatal(got)
	}
	if err.File != "" || err.Error() != "invalid input" {
		t.Fatal("wrapping changed the original diagnostic")
	}
}
