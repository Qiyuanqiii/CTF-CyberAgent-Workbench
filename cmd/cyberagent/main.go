package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cyberagent-workbench/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.ExecuteContext(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
