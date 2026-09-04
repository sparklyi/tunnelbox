package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sparklyi/tunnelbox/internal/auth"
)

func (s *Store) PasswordHash(ctx context.Context) ([]byte, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT admin_password_hash FROM workspace ORDER BY id LIMIT 1`).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get administrator password: %w", err)
	}
	return []byte(hash), nil
}

func (s *Store) SavePasswordHash(ctx context.Context, hash []byte) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE workspace SET admin_password_hash = ?, updated_at = ?
		WHERE admin_password_hash = ''`, string(hash), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save administrator password: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check administrator password: %w", err)
	}
	if affected == 0 {
		return auth.ErrAlreadySetup
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_session (token_hash, expires_at, created_at) VALUES (?, ?, ?)`,
		tokenHash, expiresAt.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("create authentication session: %w", err)
	}
	return nil
}

func (s *Store) SessionValid(ctx context.Context, tokenHash string, now time.Time) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM auth_session WHERE token_hash = ? AND expires_at > ?)`,
		tokenHash, now.UTC().Format(time.RFC3339Nano)).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check authentication session: %w", err)
	}
	return exists == 1, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM auth_session WHERE token_hash = ?`, tokenHash); err != nil {
		return fmt.Errorf("delete authentication session: %w", err)
	}
	return nil
}

var _ auth.Repository = (*Store)(nil)
