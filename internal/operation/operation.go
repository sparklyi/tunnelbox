package operation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("operation not found")
	ErrConflict = errors.New("service already has an active operation")
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusUnknown   Status = "unknown"
)

type Operation struct {
	ID           string
	ServiceID    string
	Kind         string
	Status       Status
	CurrentStep  string
	Attempts     int
	ErrorCode    string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// Repository is the storage contract consumed by the operation manager and API.
type Repository interface {
	Create(context.Context, Operation) error
	Get(context.Context, string) (Operation, error)
	Update(context.Context, Operation) error
	ListIncomplete(context.Context) ([]Operation, error)
	FindActiveForService(context.Context, string) (Operation, error)
}

// Task performs one operation. It should return an error only after recording
// any remote uncertainty in its own domain-specific result.
type Task func(context.Context, Operation) error

// Failure carries the only error details that are safe to persist and expose
// through the operation API.
type Failure struct {
	Code    string
	Message string
	Unknown bool
}

func (f *Failure) Error() string {
	if f == nil || f.Message == "" {
		return "operation failed"
	}
	return f.Message
}

type Manager struct {
	repo   Repository
	mu     sync.Mutex
	active map[string]struct{}
	now    func() time.Time
}

func NewManager(repo Repository) *Manager {
	return &Manager{repo: repo, active: make(map[string]struct{}), now: time.Now}
}

func (m *Manager) Start(ctx context.Context, serviceID, kind string, task Task) (Operation, error) {
	if serviceID == "" || kind == "" || task == nil {
		return Operation{}, errors.New("service id, kind and task are required")
	}
	m.mu.Lock()
	if _, exists := m.active[serviceID]; exists {
		m.mu.Unlock()
		return Operation{}, ErrConflict
	}
	if existing, err := m.repo.FindActiveForService(ctx, serviceID); err == nil {
		m.mu.Unlock()
		return Operation{}, fmt.Errorf("%w: %s", ErrConflict, existing.ID)
	} else if !errors.Is(err, ErrNotFound) {
		m.mu.Unlock()
		return Operation{}, fmt.Errorf("check active operation: %w", err)
	}
	now := m.now().UTC()
	op := Operation{ID: newID("op"), ServiceID: serviceID, Kind: kind, Status: StatusPending, CreatedAt: now, UpdatedAt: now}
	if err := m.repo.Create(ctx, op); err != nil {
		m.mu.Unlock()
		return Operation{}, err
	}
	m.active[serviceID] = struct{}{}
	m.mu.Unlock()
	go m.run(ctx, op, task)
	return op, nil
}

func (m *Manager) run(ctx context.Context, op Operation, task Task) {
	now := m.now().UTC()
	op.Status = StatusRunning
	op.Attempts++
	op.StartedAt = &now
	op.UpdatedAt = now
	if err := m.repo.Update(context.Background(), op); err != nil {
		op.Status = StatusUnknown
		op.ErrorCode = "operation_state_unavailable"
		op.ErrorMessage = "operation state could not be updated"
		op.UpdatedAt = m.now().UTC()
		_ = m.repo.Update(context.Background(), op)
		m.release(op.ServiceID)
		return
	}
	err := task(ctx, op)
	if latest, getErr := m.repo.Get(context.Background(), op.ID); getErr == nil {
		op.CurrentStep = latest.CurrentStep
	}
	finished := m.now().UTC()
	op.UpdatedAt = finished
	op.FinishedAt = &finished
	if err != nil {
		op.Status = StatusFailed
		op.ErrorCode = "operation_failed"
		op.ErrorMessage = "operation failed"
		if errors.Is(err, context.Canceled) {
			op.Status = StatusUnknown
			op.ErrorCode = "operation_canceled"
			op.ErrorMessage = "operation was canceled before its result was known"
		}
		var failure *Failure
		if errors.As(err, &failure) && failure != nil {
			if failure.Code != "" {
				op.ErrorCode = failure.Code
			}
			if failure.Message != "" {
				op.ErrorMessage = failure.Message
			}
			if failure.Unknown {
				op.Status = StatusUnknown
			}
		}
	} else {
		op.Status = StatusSucceeded
		op.CurrentStep = "complete"
	}
	if updateErr := m.repo.Update(context.Background(), op); updateErr != nil {
		// The remote work may have completed even when its local status write failed.
		op.Status = StatusUnknown
		op.ErrorCode = "operation_state_unavailable"
		op.ErrorMessage = "operation result could not be persisted"
		_ = m.repo.Update(context.Background(), op)
	}
	m.release(op.ServiceID)
}

func (m *Manager) Get(ctx context.Context, id string) (Operation, error) {
	return m.repo.Get(ctx, id)
}

func (m *Manager) SetStep(ctx context.Context, id, step string) error {
	if id == "" || step == "" {
		return errors.New("operation id and step are required")
	}
	op, err := m.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if op.Status != StatusPending && op.Status != StatusRunning {
		return nil
	}
	op.CurrentStep = step
	op.UpdatedAt = m.now().UTC()
	return m.repo.Update(ctx, op)
}

func (m *Manager) Recover(ctx context.Context, resolver func(Operation) Task) error {
	operations, err := m.repo.ListIncomplete(ctx)
	if err != nil {
		return err
	}
	for _, op := range operations {
		task := resolver(op)
		if task == nil {
			op.Status = StatusUnknown
			op.ErrorCode = "operation_not_resumable"
			op.ErrorMessage = "no handler is available to resume this operation"
			op.UpdatedAt = m.now().UTC()
			if updateErr := m.repo.Update(ctx, op); updateErr != nil {
				return updateErr
			}
			continue
		}
		m.mu.Lock()
		if _, exists := m.active[op.ServiceID]; exists {
			m.mu.Unlock()
			continue
		}
		m.active[op.ServiceID] = struct{}{}
		m.mu.Unlock()
		go m.run(ctx, op, task)
	}
	return nil
}

func (m *Manager) release(serviceID string) {
	m.mu.Lock()
	delete(m.active, serviceID)
	m.mu.Unlock()
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
