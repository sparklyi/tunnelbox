package sqlite

import (
	"context"
	"fmt"
	"time"
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
