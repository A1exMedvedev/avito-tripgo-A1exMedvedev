package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/config"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/container"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "trip-service stopped with error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := container.New(ctx, cfg)
	if err != nil {
		return err
	}

	log := c.Logger()
	failed := c.Start(ctx)

	var serveErr error

	select {
	case serveErr = <-failed:
	case <-ctx.Done():
	}

	stop()
	log.InfoContext(context.Background(), "shutting down", slog.Duration("timeout", cfg.ShutdownTimeout))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	c.Shutdown(shutdownCtx)
	log.InfoContext(context.Background(), "trip-service stopped")

	return serveErr
}
