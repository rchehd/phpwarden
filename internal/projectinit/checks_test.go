package projectinit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
)

func TestInitPersistsChosenChecksAndPreservesExistingConfig(t *testing.T) {
	for _, selected := range [][]string{{"debug", "rector", "deptrac"}, {}, checker.IDs()} {
		root := drupalProject(t, ">=8.1", nil)
		calls := 0
		options := Options{CWD: root, Project: root, Runtime: "8.4", SelectChecks: func(choices []CheckChoice) ([]string, error) {
			calls++
			ids := []string{}
			for _, choice := range choices {
				ids = append(ids, choice.ID)
				if choice.ID == "rector" && !choice.Enabled {
					t.Fatal("Rector was not selected by default")
				}
			}
			if !slices.Equal(ids, checker.IDs()) {
				t.Fatalf("init choices differ from registered checks: %v", ids)
			}
			return selected, nil
		}}
		result, err := Run(context.Background(), options)
		if err != nil || !result.Created || calls != 1 {
			t.Fatalf("selection did not run once: %+v %v calls=%d", result, err, calls)
		}
		effective, err := config.Load(result.Location)
		if err != nil {
			t.Fatal(err)
		}
		for _, choice := range checkChoices(effective.Config.Tools) {
			if choice.Enabled != slices.Contains(selected, choice.ID) {
				t.Fatalf("lost selection: %+v", choice)
			}
		}
		defaults, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
		if err != nil {
			t.Fatal(err)
		}
		applyCheckSelection(&effective.Config.Tools, func() map[string]bool {
			flags := map[string]bool{}
			for _, choice := range checkChoices(defaults.Config.Tools) {
				flags[choice.ID] = choice.Enabled
			}
			return flags
		}())
		if !reflect.DeepEqual(effective.Config, defaults.Config) {
			t.Fatal("selection modified settings beyond enabled flags")
		}
		before, _ := os.ReadFile(result.Location.Path)
		result, err = Run(context.Background(), options)
		after, _ := os.ReadFile(result.Location.Path)
		if err != nil || result.Created || calls != 1 || string(before) != string(after) {
			t.Fatal("repeated init prompted or overwrote configuration")
		}
	}
}

func TestRejectedOrCancelledInitSelectionWritesNothing(t *testing.T) {
	for _, selected := range [][]string{{"unknown"}, {"debug", "debug"}, nil} {
		root := drupalProject(t, ">=8.1", nil)
		_, err := Run(context.Background(), Options{CWD: root, Project: root, Runtime: "8.4", SelectChecks: func([]CheckChoice) ([]string, error) {
			if selected == nil {
				return nil, context.Canceled
			}
			return selected, nil
		}})
		if err == nil || selected == nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("invalid selection accepted: %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, config.Filename)); !os.IsNotExist(err) {
			t.Fatal("cancelled or invalid selection created a configuration")
		}
	}
}
