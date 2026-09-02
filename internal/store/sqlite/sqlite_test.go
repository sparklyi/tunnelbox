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

func TestOpenMigratesSchemaV1ToV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	legacy.SetMaxOpenConns(1)
	statements := []string{
		`CREATE TABLE workspace (id TEXT PRIMARY KEY, name TEXT NOT NULL, account_id TEXT NOT NULL DEFAULT '', zone_id TEXT NOT NULL DEFAULT '', cloudflare_token_path TEXT NOT NULL DEFAULT '', admin_token_path TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE service (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL, hostname TEXT NOT NULL, origin_url TEXT NOT NULL, allow_type TEXT NOT NULL, allow_value TEXT NOT NULL, state TEXT NOT NULL, tunnel_id TEXT NOT NULL DEFAULT '', dns_record_id TEXT NOT NULL DEFAULT '', access_application_id TEXT NOT NULL DEFAULT '', access_policy_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE(workspace_id, hostname))`,
		`INSERT INTO workspace (id, name, created_at, updated_at) VALUES ('default', 'Default', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`INSERT INTO service (id, workspace_id, name, hostname, origin_url, allow_type, allow_value, state, created_at, updated_at) VALUES ('svc_legacy', 'default', 'Legacy', 'legacy.example.com', 'http://192.168.1.20:8080', 'email', 'user@example.com', 'draft', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		`PRAGMA user_version = 1`,
	}
	for _, statement := range statements {
		if _, err := legacy.Exec(statement); err != nil {
			legacy.Close()
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read migrated version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("version = %d, want %d", version, schemaVersion)
	}
	var mode string
	if err := db.QueryRow(`SELECT mode FROM service WHERE id = 'svc_legacy'`).Scan(&mode); err != nil {
		t.Fatalf("read migrated mode: %v", err)
	}
	if mode != "public" {
		t.Fatalf("migrated mode = %q, want public", mode)
	}
	var migrations int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&migrations); err != nil {
		t.Fatalf("check migration table: %v", err)
	}
	if migrations != 0 {
		t.Fatal("unexpected schema_migrations table")
	}

	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened database: %v", err)
	}
}
