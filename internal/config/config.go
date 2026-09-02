package config

import (
	"fmt"
	"net"
	"os"
	"strings"
)

type Config struct {
	ListenAddress       string
	DatabasePath        string
	WorkspaceID         string
	WorkspaceName       string
	AdminTokenFile      string
	CloudflareTokenFile string
	CloudflaredBinary   string
	CloudflaredDataDir  string
	WebDir              string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddress:       valueOr("TUNNELBOX_LISTEN", "127.0.0.1:8080"),
		DatabasePath:        valueOr("TUNNELBOX_DATABASE", "data/tunnelbox.db"),
		WorkspaceID:         valueOr("TUNNELBOX_WORKSPACE_ID", "default"),
		WorkspaceName:       valueOr("TUNNELBOX_WORKSPACE_NAME", "Default"),
		AdminTokenFile:      strings.TrimSpace(os.Getenv("TUNNELBOX_ADMIN_TOKEN_FILE")),
		CloudflareTokenFile: valueOr("TUNNELBOX_CLOUDFLARE_TOKEN_FILE", "data/cloudflare.token"),
		CloudflaredBinary:   valueOr("TUNNELBOX_CLOUDFLARED_BINARY", "cloudflared"),
		CloudflaredDataDir:  valueOr("TUNNELBOX_CLOUDFLARED_DATA_DIR", "data/cloudflared"),
		WebDir:              valueOr("TUNNELBOX_WEB_DIR", "web/dist"),
	}
	if cfg.ListenAddress == "" {
		return Config{}, fmt.Errorf("TUNNELBOX_LISTEN must not be empty")
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("TUNNELBOX_DATABASE must not be empty")
	}
	if cfg.WorkspaceID == "" || cfg.WorkspaceName == "" {
		return Config{}, fmt.Errorf("workspace id and name must not be empty")
	}
	if cfg.CloudflareTokenFile == "" || cfg.CloudflaredBinary == "" || cfg.CloudflaredDataDir == "" {
		return Config{}, fmt.Errorf("cloudflare token file, cloudflared binary and data directory must not be empty")
	}
	if !isLoopbackAddress(cfg.ListenAddress) && cfg.AdminTokenFile == "" {
		return Config{}, fmt.Errorf("TUNNELBOX_ADMIN_TOKEN_FILE is required for non-loopback listen address")
	}
	return cfg, nil
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func valueOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
