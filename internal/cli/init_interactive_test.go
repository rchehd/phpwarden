package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/projectinit"
)

func TestInitSelectionKeepsDefaultsAndAppliesToggles(t *testing.T) {
	defaults := []projectinit.CheckChoice{{ID: "debug", Enabled: true}, {ID: "deptrac"}, {ID: "rector", Enabled: true}}
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"\n", []string{"debug", "rector"}},
		{"1,2\n\n", []string{"deptrac", "rector"}},
		{"rector\n\n", []string{"debug"}},
		{"none\ndebug,rector\n\n", []string{"debug", "rector"}},
		{"all\n\n", []string{"debug", "deptrac", "rector"}},
		{"none\n\n", []string{}},
		{"none\ndefaults\n\n", []string{"debug", "rector"}},
		{"1,invalid\n\n", []string{"debug", "rector"}},
	} {
		var output bytes.Buffer
		got, err := chooseInitChecks(context.Background(), strings.NewReader(tc.input), &output, defaults)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("input %q: %v %v; want %v", tc.input, got, err, tc.want)
		}
		if !strings.Contains(output.String(), "[x]") || !defaults[0].Enabled || defaults[1].Enabled || !defaults[2].Enabled {
			t.Fatal("prompt lost selection markers or mutated defaults")
		}
	}
	for _, id := range checker.IDs() {
		if initCheckDescriptions[id] == "" {
			t.Errorf("missing init explanation for %s", id)
		}
	}
}

func TestInitSelectionCancellationNeverAcceptsDefaults(t *testing.T) {
	for _, input := range []string{"", "q\n", "none\n"} {
		if _, err := chooseInitChecks(context.Background(), strings.NewReader(input), io.Discard, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("input %q did not cancel: %v", input, err)
		}
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := chooseInitChecks(ctx, reader, io.Discard, nil)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled init blocked on stdin")
	}
}

func TestInitDoesNotPromptForPipesJSONCIOrNoInteraction(t *testing.T) {
	var output bytes.Buffer
	if interactiveInit(options{format: "text"}, strings.NewReader(""), &output, &output) {
		t.Fatal("redirected streams prompted")
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	for _, args := range []options{{format: "text"}, {format: "json"}, {format: "text", noInteraction: true}} {
		if interactiveInit(args, null, null, null) {
			t.Fatal("noninteractive init prompted")
		}
	}
	t.Setenv("CI", "true")
	if interactiveInit(options{format: "text"}, os.Stdin, os.Stdout, os.Stderr) {
		t.Fatal("CI prompted")
	}
}
