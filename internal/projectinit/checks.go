package projectinit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
)

type CheckChoice struct {
	ID      string
	Enabled bool
}

func checkChoices(tools config.Tools) []CheckChoice {
	value := reflect.ValueOf(tools)
	choices := make([]CheckChoice, 0, value.NumField())
	for i := range value.NumField() {
		id, _, _ := strings.Cut(value.Type().Field(i).Tag.Get("json"), ",")
		choices = append(choices, CheckChoice{ID: id, Enabled: value.Field(i).FieldByName("Enabled").Bool()})
	}
	slices.SortFunc(choices, func(a, b CheckChoice) int { return strings.Compare(a.ID, b.ID) })
	return choices
}

func checkSelection(choices []CheckChoice, selected []string) (map[string]bool, error) {
	if err := checker.ValidateOnly(selected); err != nil {
		return nil, err
	}
	enabled := make(map[string]bool, len(choices))
	for _, choice := range choices {
		enabled[choice.ID] = slices.Contains(selected, choice.ID)
	}
	for _, id := range selected {
		if _, exists := enabled[id]; !exists {
			return nil, fmt.Errorf("check %q is unavailable in this profile", id)
		}
	}
	return enabled, nil
}

func applyCheckSelection(tools *config.Tools, enabled map[string]bool) {
	value := reflect.ValueOf(tools).Elem()
	for i := range value.NumField() {
		id, _, _ := strings.Cut(value.Type().Field(i).Tag.Get("json"), ",")
		if flag, exists := enabled[id]; exists {
			value.Field(i).FieldByName("Enabled").SetBool(flag)
		}
	}
}
