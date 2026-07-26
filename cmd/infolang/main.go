// Command infolang is the InfoLang semantic-memory CLI. It is a thin wrapper
// around the internal/cli package so the command logic stays unit-testable.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/InfoLang-Inc/infolang-go/internal/cli"
)

func main() {
	// Cancel in-flight requests on Ctrl-C / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
