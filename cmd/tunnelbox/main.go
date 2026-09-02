package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/sparklyi/tunnelbox/internal/bootstrap"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bootstrap.Run(ctx, logger); err != nil {
		if !errors.Is(err, context.Canceled) {
			logger.Error("tunnelbox stopped", "error", err)
		}
		os.Exit(1)
	}
}
