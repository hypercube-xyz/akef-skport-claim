package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hypercube-xyz/akef-skport-claim/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cli.Execute(ctx, os.Args[1:])
}
