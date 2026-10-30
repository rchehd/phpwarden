package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/rchehd/phpwarden/internal/distribution"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected manifest, package or plugins")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	output := flags.String("output", "", "output file or directory")
	switch args[0] {
	case "plugins":
		version := flags.String("version", "", "release or development version")
		commit := flags.String("commit", "", "exact source Git commit")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *output == "" || *version == "" || *commit == "" {
			return fmt.Errorf("plugins requires --version, --commit and --output")
		}
		return distribution.PluginPackages(*version, *commit, *output)
	case "manifest":
		version := flags.String("version", "", "release version")
		images := flags.String("images", "", "runtime-to-digest JSON file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *output == "" || *images == "" {
			return fmt.Errorf("manifest requires --version, --images, --output")
		}
		data, err := os.ReadFile(*images)
		if err != nil {
			return err
		}
		var refs map[string]string
		if err := json.Unmarshal(data, &refs); err != nil {
			return err
		}
		data, err = toolset.PublishedManifest(*version, refs)
		if err != nil {
			return err
		}
		return os.WriteFile(*output, data, 0o644)
	case "package":
		binary := flags.String("binary", "", "built PHPWarden executable")
		readme := flags.String("readme", "README.md", "README path")
		manifest := flags.String("manifest", "", "published compatibility manifest")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *output == "" || *binary == "" {
			return fmt.Errorf("package requires --binary and --output")
		}
		return distribution.Package(context.Background(), *binary, *readme, *manifest, *output)
	default:
		return fmt.Errorf("unknown distribution command %q", args[0])
	}
}
