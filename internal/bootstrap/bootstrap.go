package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sparklyi/tunnelbox/internal/auth"
	"github.com/sparklyi/tunnelbox/internal/config"
	"github.com/sparklyi/tunnelbox/internal/httpapi"
	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
	"github.com/sparklyi/tunnelbox/internal/store/sqlite"
)

const defaultWorkspaceID = "default"

// Run assembles the application and serves until ctx is canceled.
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

	store := sqlite.NewStore(db)
	workspaceID := cfg.WorkspaceID
	if workspaceID == "" {
		workspaceID = defaultWorkspaceID
	}
	if err := store.EnsureWorkspace(ctx, workspaceID, cfg.WorkspaceName); err != nil {
		return fmt.Errorf("ensure workspace: %w", err)
	}
	adminToken, err := auth.LoadToken(cfg.AdminTokenFile)
	if err != nil {
		return fmt.Errorf("load admin token: %w", err)
	}
	services := service.NewUseCase(store.Services(), workspaceID)
	operations := operation.NewManager(store.Operations())
	router, err := httpapi.NewRouter(httpapi.Dependencies{
		Services: services, Operations: operations, AdminToken: adminToken, Logger: logger,
		Readiness: db.PingContext,
	})
	if err != nil {
		return fmt.Errorf("build http router: %w", err)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           router,
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
