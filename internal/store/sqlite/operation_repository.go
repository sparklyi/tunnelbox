package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sparklyi/tunnelbox/internal/operation"
)

func (s *OperationRepository) Create(ctx context.Context, item operation.Operation) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO operation (
			id, service_id, kind, status, current_step, attempts, error_code, error_message,
			created_at, updated_at, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.ServiceID, item.Kind, item.Status, item.CurrentStep, item.Attempts,
		item.ErrorCode, item.ErrorMessage, formatTime(item.CreatedAt), formatTime(item.UpdatedAt),
		formatOptionalTime(item.StartedAt), formatOptionalTime(item.FinishedAt))
	if err != nil {
		return fmt.Errorf("create operation: %w", err)
	}
	return nil
}

func (s *OperationRepository) Get(ctx context.Context, id string) (operation.Operation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, service_id, kind, status, current_step, attempts, error_code, error_message,
		       created_at, updated_at, started_at, finished_at
		FROM operation WHERE id = ?`, id)
	item, err := scanOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operation.Operation{}, operation.ErrNotFound
	}
	if err != nil {
		return operation.Operation{}, fmt.Errorf("get operation: %w", err)
	}
	return item, nil
}

func (s *OperationRepository) Update(ctx context.Context, item operation.Operation) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE operation
		SET kind = ?, status = ?, current_step = ?, attempts = ?, error_code = ?, error_message = ?,
		    updated_at = ?, started_at = ?, finished_at = ?
		WHERE id = ?`, item.Kind, item.Status, item.CurrentStep, item.Attempts, item.ErrorCode, item.ErrorMessage,
		formatTime(item.UpdatedAt), formatOptionalTime(item.StartedAt), formatOptionalTime(item.FinishedAt), item.ID)
	if err != nil {
		return fmt.Errorf("update operation: %w", err)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return operation.ErrNotFound
	}
	return nil
}

func (s *OperationRepository) ListIncomplete(ctx context.Context) ([]operation.Operation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, service_id, kind, status, current_step, attempts, error_code, error_message,
		       created_at, updated_at, started_at, finished_at
		FROM operation WHERE status IN (?, ?) ORDER BY created_at, id`, operation.StatusPending, operation.StatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list incomplete operations: %w", err)
	}
	defer rows.Close()
	items := make([]operation.Operation, 0)
	for rows.Next() {
		item, scanErr := scanOperation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan operation: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operations: %w", err)
	}
	return items, nil
}

func (s *OperationRepository) FindActiveForService(ctx context.Context, serviceID string) (operation.Operation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, service_id, kind, status, current_step, attempts, error_code, error_message,
		       created_at, updated_at, started_at, finished_at
		FROM operation WHERE service_id = ? AND status IN (?, ?) ORDER BY created_at DESC LIMIT 1`,
		serviceID, operation.StatusPending, operation.StatusRunning)
	item, err := scanOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operation.Operation{}, operation.ErrNotFound
	}
	if err != nil {
		return operation.Operation{}, fmt.Errorf("find active operation: %w", err)
	}
	return item, nil
}

func scanOperation(row scanner) (operation.Operation, error) {
	var item operation.Operation
	var status string
	var createdAt, updatedAt string
	var startedAt, finishedAt sql.NullString
	err := row.Scan(&item.ID, &item.ServiceID, &item.Kind, &status, &item.CurrentStep, &item.Attempts,
		&item.ErrorCode, &item.ErrorMessage, &createdAt, &updatedAt, &startedAt, &finishedAt)
	if err != nil {
		return operation.Operation{}, err
	}
	item.Status = operation.Status(status)
	item.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return operation.Operation{}, fmt.Errorf("parse created_at: %w", err)
	}
	item.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return operation.Operation{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if startedAt.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, startedAt.String)
		if parseErr != nil {
			return operation.Operation{}, fmt.Errorf("parse started_at: %w", parseErr)
		}
		item.StartedAt = &value
	}
	if finishedAt.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, finishedAt.String)
		if parseErr != nil {
			return operation.Operation{}, fmt.Errorf("parse finished_at: %w", parseErr)
		}
		item.FinishedAt = &value
	}
	return item, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
