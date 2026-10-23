package runrequest

import (
	"fmt"
	"slices"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/phpcs"
	"github.com/rchehd/phpwarden/internal/checker/twigcsfixer"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

type FixResponse struct {
	Report report.Report `json:"report"`
	Edits  []edit.Edit   `json:"edits"`
}

func FixIDs(cfg config.Config, only []string) ([]string, error) {
	if err := checker.ValidateOnly(only); err != nil {
		return nil, err
	}
	for _, id := range only {
		if id != "phpcs" && id != "twig-cs-fixer" {
			return nil, fmt.Errorf("checker %q does not support --fix; supported: phpcs, twig-cs-fixer", id)
		}
	}
	result := []string{}
	if checker.Enabled("phpcs", cfg.Tools.PHPCS.Enabled, only) {
		result = append(result, "phpcs")
	}
	if checker.Enabled("twig-cs-fixer", cfg.Tools.TwigCSFixer.Enabled, only) {
		result = append(result, "twig-cs-fixer")
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no fix-capable checker is enabled; select --only phpcs or --only twig-cs-fixer")
	}
	return result, nil
}

func (request Request) FixInputs() map[string]string {
	result := map[string]string{}
	if slices.Contains(request.Only, "phpcs") {
		for _, name := range phpcs.Select(request.Selection, request.Config.Tools.PHPCS, request.Config.Profile).Files {
			result[name] = request.Fingerprint["file:"+name]
		}
	}
	if slices.Contains(request.Only, "twig-cs-fixer") {
		for _, name := range twigcsfixer.Select(request.Selection, request.Config.Tools.TwigCSFixer, request.Config.Profile).Files {
			result[name] = request.Fingerprint["file:"+name]
		}
	}
	return result
}

func Verification(options scope.Options, selected scope.Selection) scope.Options {
	if len(selected.Files) == 0 {
		return options
	}
	options.Mode, options.Base, options.WholeFiles, options.Commits = "paths", "", false, 0
	options.CWD, options.Paths = options.Root, slices.Clone(selected.Files)
	return options
}
