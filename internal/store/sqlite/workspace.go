package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sparklyi/tunnelbox/internal/service"
)

func (s *Store) EnsureWorkspace(ctx context.Context, id, name string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspace (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, updated_at = excluded.updated_at`,
		id, name, now, now)
	if err != nil {
		return fmt.Errorf("ensure workspace: %w", err)
	}
	return nil
}

func (s *Store) GetWorkspace(ctx context.Context, id string) (service.Workspace, error) {
	var workspace service.Workspace
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, account_id, zone_id, cloudflare_token_path, admin_token_path
		FROM workspace WHERE id = ?`, id).Scan(&workspace.ID, &workspace.Name, &workspace.AccountID, &workspace.ZoneID,
		&workspace.CloudflareTokenPath, &workspace.AdminTokenPath)
	if errors.Is(err, sql.ErrNoRows) {
		return service.Workspace{}, service.ErrNotFound
	}
	if err != nil {
		return service.Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return workspace, nil
}

func (s *Store) SaveCloudflareConfig(ctx context.Context, workspaceID, accountID, zoneID, tokenPath string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE workspace SET account_id = ?, zone_id = ?, cloudflare_token_path = ?, updated_at = ?
		WHERE id = ?`, accountID, zoneID, tokenPath, time.Now().UTC().Format(time.RFC3339Nano), workspaceID)
	if err != nil {
		return fmt.Errorf("save cloudflare config: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return service.ErrNotFound
	}
	return nil
}

var _ service.WorkspaceRepository = (*Store)(nil)
