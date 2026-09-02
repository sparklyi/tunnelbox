package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadTokenUsesOwnerOnlyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "admin.token")
	if err := SaveToken(path, "secret-value"); err != nil {
		t.Fatalf("save: %v", err)
	}
	value, err := LoadToken(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if value != "secret-value" {
		t.Fatalf("value = %q", value)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}
