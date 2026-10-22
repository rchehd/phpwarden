package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rchehd/phpwarden/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.RunContext(ctx, os.Args[1:], cwd, os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
