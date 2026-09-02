package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sparklyi/tunnelbox/internal/service"
	modernsqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func (s *ServiceRepository) List(ctx context.Context, workspaceID string) ([]service.Service, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, mode, hostname, origin_url, allow_type, allow_value,
		       state, tunnel_id, private_route_id, dns_record_id, access_application_id, access_policy_id, public_url,
		       created_at, updated_at
		FROM service WHERE workspace_id = ? ORDER BY created_at, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer rows.Close()

	services := make([]service.Service, 0)
	for rows.Next() {
		item, scanErr := scanService(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan service: %w", scanErr)
		}
		services = append(services, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate services: %w", err)
	}
	return services, nil
}

func (s *ServiceRepository) Get(ctx context.Context, workspaceID, id string) (service.Service, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, mode, hostname, origin_url, allow_type, allow_value,
		       state, tunnel_id, private_route_id, dns_record_id, access_application_id, access_policy_id, public_url,
		       created_at, updated_at
		FROM service WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	item, err := scanService(row)
	if errors.Is(err, sql.ErrNoRows) {
		return service.Service{}, service.ErrNotFound
	}
	if err != nil {
		return service.Service{}, fmt.Errorf("get service: %w", err)
	}
	return item, nil
}

func (s *ServiceRepository) Create(ctx context.Context, item service.Service) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO service (
			id, workspace_id, name, mode, hostname, origin_url, allow_type, allow_value, state,
			tunnel_id, private_route_id, dns_record_id, access_application_id, access_policy_id, public_url, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.WorkspaceID, item.Name, storedMode(item.Mode), item.Hostname, item.OriginURL, item.AllowType,
		item.AllowValue, item.State, item.TunnelID, item.PrivateRouteID, item.DNSRecordID, item.AccessApplicationID,
		item.AccessPolicyID, item.PublicURL, item.CreatedAt.UTC().Format(time.RFC3339Nano), item.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if isConstraintError(err) {
			return service.ErrConflict
		}
		return fmt.Errorf("create service: %w", err)
	}
	return nil
}

func (s *ServiceRepository) Update(ctx context.Context, item service.Service) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE service
		SET name = ?, mode = ?, hostname = ?, origin_url = ?, allow_type = ?, allow_value = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?`,
		item.Name, storedMode(item.Mode), item.Hostname, item.OriginURL, item.AllowType, item.AllowValue,
		item.UpdatedAt.UTC().Format(time.RFC3339Nano), item.WorkspaceID, item.ID)
	if err != nil {
		if isConstraintError(err) {
			return service.ErrConflict
		}
		return fmt.Errorf("update service: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return service.ErrNotFound
	}
	return nil
}

func isConstraintError(err error) bool {
	var sqliteErr *modernsqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
}

func (s *ServiceRepository) Delete(ctx context.Context, workspaceID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM service WHERE workspace_id = ? AND id = ?`, workspaceID, id)
	if err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return service.ErrNotFound
	}
	return nil
}

func (s *ServiceRepository) SetState(ctx context.Context, workspaceID, id string, state service.State) error {
	result, err := s.db.ExecContext(ctx, `UPDATE service SET state = ?, updated_at = ? WHERE workspace_id = ? AND id = ?`,
		state, time.Now().UTC().Format(time.RFC3339Nano), workspaceID, id)
	if err != nil {
		return fmt.Errorf("set service state: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return service.ErrNotFound
	}
	return nil
}

func (s *ServiceRepository) SetRemoteRefs(ctx context.Context, workspaceID, id string, refs service.RemoteRefs) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE service SET tunnel_id = ?, private_route_id = ?, dns_record_id = ?, access_application_id = ?, access_policy_id = ?, public_url = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?`, refs.TunnelID, refs.PrivateRouteID, refs.DNSRecordID, refs.AccessApplicationID,
		refs.AccessPolicyID, refs.PublicURL, time.Now().UTC().Format(time.RFC3339Nano), workspaceID, id)
	if err != nil {
		return fmt.Errorf("set service remote refs: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return service.ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanService(row scanner) (service.Service, error) {
	var item service.Service
	var mode, allowType, state string
	var createdAt, updatedAt string
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.Name, &mode, &item.Hostname, &item.OriginURL, &allowType, &item.AllowValue,
		&state, &item.TunnelID, &item.PrivateRouteID, &item.DNSRecordID, &item.AccessApplicationID, &item.AccessPolicyID, &item.PublicURL, &createdAt, &updatedAt)
	if err != nil {
		return service.Service{}, err
	}
	item.Mode = storedMode(service.Mode(mode))
	item.AllowType = service.AllowType(allowType)
	item.State = service.State(state)
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return service.Service{}, fmt.Errorf("parse created_at: %w", err)
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return service.Service{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return item, nil
}

func storedMode(mode service.Mode) service.Mode {
	if mode == "" {
		return service.ModePublic
	}
	return mode
}
