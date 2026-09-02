package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenCreatesSchemaAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tunnelbox.db")
	ctx := context.Background()

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close()

	db, err = Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("version = %d, want %d", version, schemaVersion)
	}

	var tableName string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'service'`).Scan(&tableName); err != nil {
		t.Fatalf("service table: %v", err)
	}
	if tableName != "service" {
		t.Fatalf("table = %q", tableName)
	}

	var _ *sql.DB = db
}
