package operation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryRepository struct {
	mu    sync.Mutex
	items map[string]Operation
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{items: make(map[string]Operation)}
}

func (r *memoryRepository) Create(_ context.Context, item Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[item.ID]; exists {
		return errors.New("duplicate")
	}
	r.items[item.ID] = item
	return nil
}

func (r *memoryRepository) Get(_ context.Context, id string) (Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return Operation{}, ErrNotFound
	}
	return item, nil
}

func (r *memoryRepository) Update(_ context.Context, item Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}

func (r *memoryRepository) ListIncomplete(_ context.Context) ([]Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]Operation, 0)
	for _, item := range r.items {
		if item.Status == StatusPending || item.Status == StatusRunning {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *memoryRepository) FindActiveForService(_ context.Context, serviceID string) (Operation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.items {
		if item.ServiceID == serviceID && (item.Status == StatusPending || item.Status == StatusRunning) {
			return item, nil
		}
	}
	return Operation{}, ErrNotFound
}

func TestManagerRunsOnceAndSanitizesFailure(t *testing.T) {
	repo := newMemoryRepository()
	manager := NewManager(repo)
	started := make(chan struct{})
	release := make(chan struct{})
	op, err := manager.Start(context.Background(), "svc_1", "deploy", func(_ context.Context, _ Operation) error {
		close(started)
		<-release
		return errors.New("secret token must not be persisted")
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-started
	if _, err := manager.Start(context.Background(), "svc_1", "deploy", func(context.Context, Operation) error { return nil }); !errors.Is(err, ErrConflict) {
		t.Fatalf("second start error = %v, want conflict", err)
	}
	close(release)

	deadline := time.After(2 * time.Second)
	for {
		current, getErr := manager.Get(context.Background(), op.ID)
		if getErr != nil {
			t.Fatalf("get: %v", getErr)
		}
		if current.Status == StatusFailed {
			if current.ErrorMessage != "operation failed" || current.ErrorCode != "operation_failed" {
				t.Fatalf("unsanitized failure: %+v", current)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("operation did not finish: %+v", current)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestFailureCanMarkUnknown(t *testing.T) {
	repo := newMemoryRepository()
	manager := NewManager(repo)
	op, err := manager.Start(context.Background(), "svc_2", "deploy", func(context.Context, Operation) error {
		return &Failure{Code: "remote_state_unknown", Message: "remote state is unknown", Unknown: true}
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.After(2 * time.Second)
	for {
		current, getErr := manager.Get(context.Background(), op.ID)
		if getErr != nil {
			t.Fatalf("get: %v", getErr)
		}
		if current.Status == StatusUnknown {
			if current.ErrorCode != "remote_state_unknown" {
				t.Fatalf("unexpected unknown operation: %+v", current)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("operation did not finish: %+v", current)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
