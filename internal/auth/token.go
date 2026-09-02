package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrInvalidToken = errors.New("invalid administrator token")

func LoadToken(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat admin token file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("admin token file must be accessible only by its owner: %w", ErrInvalidToken)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read admin token file: %w", err)
	}
	token := strings.TrimSpace(string(value))
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return "", ErrInvalidToken
	}
	return token, nil
}

func SaveToken(path, token string) error {
	path = strings.TrimSpace(path)
	token = strings.TrimSpace(token)
	if path == "" || token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return ErrInvalidToken
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".admin-token-*")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("chmod temporary token file: %w", err)
	}
	if _, err := temporary.WriteString(token + "\n"); err != nil {
		temporary.Close()
		return fmt.Errorf("write token file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace token file: %w", err)
	}
	return nil
}
