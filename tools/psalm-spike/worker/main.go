package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/worker"
)

func main() {
	project := flag.String("project", "", "Installed disposable Drupal fixture")
	bundle := flag.String("bundle", "", "Installed isolated Psalm vendor")
	php := flag.String("php", "php", "Native PHP executable")
	flag.Parse()
	location, err := config.Resolve(config.LocationOptions{Project: *project})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	effective, err := config.Load(location)
	if err == nil {
		effective, err = discovery.Apply(location, effective)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	result := worker.AnalyzePsalm(context.Background(), location, effective, worker.PsalmOptions{Bundle: *bundle, PHP: *php, MemoryMB: 1536}, runner.Exec{})
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if !result.Complete {
		os.Exit(3)
	}
}
