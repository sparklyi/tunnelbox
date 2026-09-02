package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sparklyi/tunnelbox/internal/config"
	"github.com/sparklyi/tunnelbox/internal/store/sqlite"
)

// Run assembles the application. Feature wiring is added in later slices.
func Run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           http.NewServeMux(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Error("http server shutdown failed", "error", shutdownErr)
		}
	}()

	logger.Info("tunnelbox listening", "address", cfg.ListenAddress, "database", cfg.DatabasePath)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
