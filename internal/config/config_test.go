package config

import "testing"

func TestLoadParsesSecureCookies(t *testing.T) {
	t.Setenv("TUNNELBOX_COOKIE_SECURE", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.SecureCookies {
		t.Fatal("secure cookies disabled, want enabled")
	}
}

func TestLoadRejectsInvalidSecureCookies(t *testing.T) {
	t.Setenv("TUNNELBOX_COOKIE_SECURE", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("invalid secure cookie setting unexpectedly accepted")
	}
}
