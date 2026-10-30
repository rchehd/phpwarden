package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/psalmconfig"
	"github.com/rchehd/phpwarden/internal/runner"
)

func main() {
	root := flag.String("project", "", "Disposable fixture root")
	directory := flag.String("directory", "", "Private prepared workspace")
	bundle := flag.String("bundle", "", "Pinned schema vendor root")
	php := flag.String("php", "php", "Native PHP executable")
	baseline := flag.String("baseline", "", "Explicit project-relative taint baseline")
	flag.Parse()
	fail := func(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(3) }
	if *root == "" || *directory == "" || *bundle == "" {
		fail(fmt.Errorf("project, directory and bundle are required"))
	}
	preparedDir := filepath.Join(*directory, "prepared")
	if err := os.Mkdir(preparedDir, 0o700); err != nil {
		fail(err)
	}
	base, err := os.ReadFile(filepath.Join(*root, "profile-base.xml"))
	if err != nil {
		fail(err)
	}
	config := filepath.Join(preparedDir, "psalm.xml")
	if err := os.WriteFile(config, base, 0o600); err != nil {
		fail(err)
	}
	entrypoints, err := os.ReadFile(filepath.Join(*root, "generated.php"))
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(preparedDir, "entrypoints.php"), entrypoints, 0o600); err != nil {
		fail(err)
	}
	native, err := psalmconfig.Load(*root, ".", psalmconfig.Options{Discover: true, Baseline: *baseline})
	if err != nil {
		fail(err)
	}
	prepared, err := native.Apply(context.Background(), *root, *bundle, *directory, *php, 512, psalmanalysis.Prepared{Config: config, Inputs: map[string]string{}}, runner.Exec{})
	if err != nil {
		fail(err)
	}
	if prepared.ReadinessConfig == "" {
		prepared.ReadinessConfig = prepared.Config
	}
	if err := json.NewEncoder(os.Stdout).Encode(prepared); err != nil {
		fail(err)
	}
}
