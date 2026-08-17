package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ZheglY/family-tree-identity-service/internal/app"
	"github.com/ZheglY/family-tree-identity-service/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "load identity service config:", err)
		os.Exit(1)
	}

	if err := app.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "identity service failed:", err)
		os.Exit(1)
	}
}
