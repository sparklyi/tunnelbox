package operation

import (
	"context"
	"errors"
	"fmt"
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

func TestShutdownCancelsAndWaitsWithoutUsingRequestContext(t *testing.T) {
	repo := newMemoryRepository()
	manager := NewManager(repo)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})

	op, err := manager.Start(requestCtx, "svc_shutdown", "deploy", func(ctx context.Context, _ Operation) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	awaitSignal(t, started, "operation start")
	cancelRequest()
	select {
	case <-canceled:
		t.Fatal("request cancellation stopped an accepted operation")
	case <-time.After(25 * time.Millisecond):
	}

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- manager.Shutdown(context.Background())
	}()
	awaitSignal(t, canceled, "application context cancellation")
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before task exit: %v", err)
	default:
	}
	close(release)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	if _, err := manager.Start(context.Background(), "svc_after", "deploy", func(context.Context, Operation) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Fatalf("start after shutdown error = %v, want %v", err, ErrClosed)
	}
	final, err := manager.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatalf("read final operation: %v", err)
	}
	if final.Status != StatusUnknown || final.ErrorCode != "operation_canceled" || final.ErrorMessage == "" {
		t.Fatalf("final operation = %+v", final)
	}
}

func TestStartAndShutdownAreConcurrentSafe(t *testing.T) {
	repo := newMemoryRepository()
	manager := NewManager(repo)
	start := make(chan struct{})
	errorsByCall := make(chan error, 25)
	var callers sync.WaitGroup
	for index := range 24 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			<-start
			_, err := manager.Start(context.Background(), fmt.Sprintf("svc_%d", index), "deploy", func(ctx context.Context, _ Operation) error {
				<-ctx.Done()
				return ctx.Err()
			})
			errorsByCall <- err
		}()
	}
	callers.Add(1)
	go func() {
		defer callers.Done()
		<-start
		errorsByCall <- manager.Shutdown(context.Background())
	}()
	close(start)
	callers.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		if err != nil && !errors.Is(err, ErrClosed) {
			t.Fatalf("concurrent call error = %v", err)
		}
	}
}

func TestRecoverUsesManagerLifecycle(t *testing.T) {
	repo := newMemoryRepository()
	now := time.Now().UTC()
	op := Operation{ID: "op_recover", ServiceID: "svc_recover", Kind: "deploy", Status: StatusPending, CreatedAt: now, UpdatedAt: now}
	repo.items[op.ID] = op
	manager := NewManager(repo)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	started := make(chan struct{})
	canceled := make(chan struct{})

	if err := manager.Recover(requestCtx, func(Operation) Task {
		return func(ctx context.Context, _ Operation) error {
			close(started)
			<-ctx.Done()
			close(canceled)
			return ctx.Err()
		}
	}); err != nil {
		t.Fatalf("recover: %v", err)
	}
	awaitSignal(t, started, "recovered operation start")
	cancelRequest()
	select {
	case <-canceled:
		t.Fatal("request cancellation stopped a recovered operation")
	case <-time.After(25 * time.Millisecond):
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	awaitSignal(t, canceled, "recovered operation cancellation")
}

func TestShutdownHonorsDeadline(t *testing.T) {
	manager := NewManager(newMemoryRepository())
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := manager.Start(context.Background(), "svc_deadline", "deploy", func(context.Context, Operation) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatalf("start: %v", err)
	}
	awaitSignal(t, started, "operation start")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := manager.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	close(release)
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("wait for shutdown: %v", err)
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
