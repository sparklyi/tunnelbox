package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/sparklyi/tunnelbox/internal/bootstrap"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := bootstrap.Run(context.Background(), logger); err != nil {
		if !errors.Is(err, context.Canceled) {
			logger.Error("tunnelbox stopped", "error", err)
		}
		os.Exit(1)
	}
}
