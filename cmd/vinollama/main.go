package main

import (
	"context"
	"os"
	"os/signal"

	"vinollama/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
