package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 5

const schema = `
CREATE TABLE workspace (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	account_id TEXT NOT NULL DEFAULT '',
	zone_id TEXT NOT NULL DEFAULT '',
	cloudflare_token_path TEXT NOT NULL DEFAULT '',
	admin_password_hash TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE service (
	id TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	mode TEXT NOT NULL,
	hostname TEXT NOT NULL DEFAULT '',
	origin_url TEXT NOT NULL,
	allow_type TEXT NOT NULL DEFAULT '',
	allow_value TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL,
	tunnel_id TEXT NOT NULL DEFAULT '',
	private_route_id TEXT NOT NULL DEFAULT '',
	dns_record_id TEXT NOT NULL DEFAULT '',
	access_application_id TEXT NOT NULL DEFAULT '',
	access_policy_id TEXT NOT NULL DEFAULT '',
	public_url TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX service_workspace_hostname ON service(workspace_id, hostname) WHERE hostname <> '';
CREATE INDEX service_workspace ON service(workspace_id);
CREATE TABLE operation (
	id TEXT PRIMARY KEY,
	service_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	current_step TEXT NOT NULL DEFAULT '',
	attempts INTEGER NOT NULL DEFAULT 0,
	error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	started_at TEXT,
	finished_at TEXT
);
CREATE INDEX operation_service_status ON operation(service_id, status);
CREATE TABLE auth_session (
	token_hash TEXT PRIMARY KEY,
	expires_at TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX auth_session_expires_at ON auth_session(expires_at);
PRAGMA user_version = 5;
`

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	if err := discardObsoleteDatabase(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database: %w", err)
	}

	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		return nil, fmt.Errorf("read database version: %w", err)
	}
	if version == 0 {
		if _, err := db.ExecContext(ctx, schema); err != nil {
			db.Close()
			return nil, fmt.Errorf("create database schema: %w", err)
		}
	}
	return db, nil
}

func discardObsoleteDatabase(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat database: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("inspect database: %w", err)
	}
	var version int
	err = db.QueryRow(`PRAGMA user_version`).Scan(&version)
	closeErr := db.Close()
	if err != nil {
		return fmt.Errorf("read database version: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close database: %w", closeErr)
	}
	if version != schemaVersion {
		return removeDatabase(path)
	}
	return nil
}

func removeDatabase(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove obsolete database: %w", err)
		}
	}
	return nil
}
