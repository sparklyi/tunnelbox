package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
)

func TestOpenCreatesSchemaAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tunnelbox.db")
	ctx := context.Background()

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

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

	for table, wantColumns := range map[string][]string{
		"workspace":    {"id", "name", "account_id", "zone_id", "cloudflare_token_path", "admin_password_hash", "created_at", "updated_at"},
		"service":      {"id", "workspace_id", "name", "mode", "hostname", "origin_url", "allow_type", "allow_value", "state", "tunnel_id", "private_route_id", "dns_record_id", "access_application_id", "access_policy_id", "public_url", "created_at", "updated_at"},
		"operation":    {"id", "service_id", "kind", "status", "current_step", "attempts", "error_code", "error_message", "created_at", "updated_at", "started_at", "finished_at"},
		"auth_session": {"token_hash", "expires_at", "created_at"},
	} {
		gotColumns := tableColumns(t, db, table)
		if !slices.Equal(gotColumns, wantColumns) {
			t.Fatalf("%s columns = %v, want %v", table, gotColumns, wantColumns)
		}
	}
	for _, index := range []string{"service_workspace_hostname", "service_workspace", "operation_service_status", "auth_session_expires_at"} {
		var found int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&found); err != nil {
			t.Fatalf("find index %s: %v", index, err)
		}
		if found != 1 {
			t.Fatalf("index %s not found", index)
		}
	}

	var _ *sql.DB = db
}

func TestOpenReplacesNonCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open old database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE obsolete (value TEXT); INSERT INTO obsolete VALUES ('keep me'); PRAGMA user_version = 4`); err != nil {
		db.Close()
		t.Fatalf("create old schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close old database: %v", err)
	}

	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatalf("replace old database: %v", err)
	}
	defer db.Close()

	var obsoleteTables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'obsolete'`).Scan(&obsoleteTables); err != nil {
		t.Fatalf("check obsolete table: %v", err)
	}
	if obsoleteTables != 0 {
		t.Fatal("obsolete schema was not discarded")
	}
	var services int
	if err := db.QueryRow(`SELECT count(*) FROM service`).Scan(&services); err != nil {
		t.Fatalf("count services in replacement database: %v", err)
	}
	if services != 0 {
		t.Fatalf("replacement database contains %d services", services)
	}
}

func tableColumns(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		t.Fatalf("read %s columns: %v", table, err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan %s column: %v", table, err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s columns: %v", table, err)
	}
	return columns
}
