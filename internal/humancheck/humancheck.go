// Package humancheck wires the bouncer challenge gate into tunnelbox.
//
// The key lives next to the database so that a restart does not invalidate
// tickets that are still in flight, and so that no environment variable is
// required for a self-hosted install.
package humancheck

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sparklyi/bouncer"
)

// KeySize is the master secret length expected by bouncer.
const KeySize = 32

// LoadOrCreateKey reads the master secret, generating it with 0600 permissions
// on first use.
func LoadOrCreateKey(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("human check key path must not be empty")
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		key := bytes.TrimSpace(raw)
		if len(key) < KeySize {
			return nil, fmt.Errorf("human check key %s is shorter than %d bytes", path, KeySize)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read human check key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create human check key directory: %w", err)
	}
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate human check key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return LoadOrCreateKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create human check key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write human check key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close human check key: %w", err)
	}
	return key, nil
}

// NewGate builds the challenge gate from the key file, creating the key when
// it does not exist yet.
func NewGate(path string) (*bouncer.Gate, error) {
	key, err := LoadOrCreateKey(path)
	if err != nil {
		return nil, err
	}
	return bouncer.New(bouncer.WithSecret(key))
}
