package main

import (
	"fmt"
	"os"

	"github.com/rchehd/phpwarden/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(cli.Run(os.Args[1:], cwd, os.Stdout, os.Stderr))
}
