package config

import (
	"fmt"
	"os"
)

type Config struct {
	ListenAddress string
	DatabasePath  string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddress: valueOr("TUNNELBOX_LISTEN", "127.0.0.1:8080"),
		DatabasePath:  valueOr("TUNNELBOX_DATABASE", "data/tunnelbox.db"),
	}
	if cfg.ListenAddress == "" {
		return Config{}, fmt.Errorf("TUNNELBOX_LISTEN must not be empty")
	}
	if cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("TUNNELBOX_DATABASE must not be empty")
	}
	return cfg, nil
}

func valueOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
