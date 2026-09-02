package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 2

func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}

	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, schemaVersion)
	}
	if version == schemaVersion {
		return nil
	}

	if version < 1 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration: %w", err)
		}
		defer tx.Rollback()
		statements := []string{
			`CREATE TABLE IF NOT EXISTS workspace (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				account_id TEXT NOT NULL DEFAULT '',
				zone_id TEXT NOT NULL DEFAULT '',
				cloudflare_token_path TEXT NOT NULL DEFAULT '',
				admin_token_path TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS service (
				id TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				hostname TEXT NOT NULL,
				origin_url TEXT NOT NULL,
				allow_type TEXT NOT NULL,
				allow_value TEXT NOT NULL,
				state TEXT NOT NULL,
				tunnel_id TEXT NOT NULL DEFAULT '',
				dns_record_id TEXT NOT NULL DEFAULT '',
				access_application_id TEXT NOT NULL DEFAULT '',
				access_policy_id TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(workspace_id, hostname)
			)`,
			`CREATE TABLE IF NOT EXISTS operation (
				id TEXT PRIMARY KEY,
				service_id TEXT NOT NULL REFERENCES service(id) ON DELETE CASCADE,
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
			)`,
			`CREATE INDEX IF NOT EXISTS idx_service_workspace ON service(workspace_id)`,
			`CREATE INDEX IF NOT EXISTS idx_operation_service_status ON operation(service_id, status)`,
			`PRAGMA user_version = 1`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migration statement: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration: %w", err)
		}
		version = 1
	}

	if version < 2 {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration: %w", err)
		}
		defer tx.Rollback()
		statements := []string{
			`ALTER TABLE service ADD COLUMN mode TEXT NOT NULL DEFAULT 'public'`,
			`ALTER TABLE service ADD COLUMN private_route_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE service ADD COLUMN public_url TEXT NOT NULL DEFAULT ''`,
			`PRAGMA user_version = 2`,
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migration statement: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration: %w", err)
		}
	}

	return nil
}
