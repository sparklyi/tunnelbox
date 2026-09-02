package service

import (
	"context"
	"errors"
	"testing"
)

type memoryRepository struct {
	items map[string]Service
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{items: make(map[string]Service)}
}

func (r *memoryRepository) List(_ context.Context, workspaceID string) ([]Service, error) {
	items := make([]Service, 0)
	for _, item := range r.items {
		if item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *memoryRepository) Get(_ context.Context, _, id string) (Service, error) {
	item, ok := r.items[id]
	if !ok {
		return Service{}, ErrNotFound
	}
	return item, nil
}

func (r *memoryRepository) Create(_ context.Context, item Service) error {
	if _, exists := r.items[item.ID]; exists {
		return ErrConflict
	}
	r.items[item.ID] = item
	return nil
}

func (r *memoryRepository) Update(_ context.Context, item Service) error {
	if _, exists := r.items[item.ID]; !exists {
		return ErrNotFound
	}
	r.items[item.ID] = item
	return nil
}

func (r *memoryRepository) Delete(_ context.Context, _, id string) error {
	if _, exists := r.items[id]; !exists {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *memoryRepository) SetState(_ context.Context, _, id string, state State) error {
	item, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	item.State = state
	r.items[id] = item
	return nil
}

func (r *memoryRepository) SetRemoteRefs(_ context.Context, _, id string, refs RemoteRefs) error {
	item, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	item.RemoteRefs = refs
	r.items[id] = item
	return nil
}

func TestUseCaseCreateNormalizesAndRejectsUnsafeOrigin(t *testing.T) {
	repo := newMemoryRepository()
	useCase := NewUseCase(repo, "workspace")
	item, err := useCase.Create(context.Background(), CreateInput{
		Name: "  Demo ", Hostname: "App.Example.COM", OriginURL: "https://127.0.0.1:8443/path",
		AllowType: " email ", AllowValue: "User@Example.COM",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Name != "Demo" || item.Hostname != "app.example.com" || item.AllowType != AllowEmail || item.AllowValue != "user@example.com" {
		t.Fatalf("unexpected normalized item: %+v", item)
	}
	if item.Mode != ModePublic {
		t.Fatalf("legacy request mode = %q, want %q", item.Mode, ModePublic)
	}
	if item.State != StateDraft || item.CreatedAt.IsZero() {
		t.Fatalf("unexpected initial state: %+v", item)
	}

	_, err = useCase.Create(context.Background(), CreateInput{
		Name: "bad", Hostname: "bad.example.com", OriginURL: "file:///tmp/x",
		AllowType: AllowEmail, AllowValue: "user@example.com",
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "origin_url" {
		t.Fatalf("error = %v, want origin validation", err)
	}
}

func TestUseCaseCreateQuickDoesNotRequireCloudflareOrAccess(t *testing.T) {
	repo := newMemoryRepository()
	useCase := NewUseCase(repo, "workspace")
	item, err := useCase.Create(context.Background(), CreateInput{
		Name: "Preview", Mode: ModeQuick, OriginURL: "http://127.0.0.1:3000",
	})
	if err != nil {
		t.Fatalf("create quick: %v", err)
	}
	if item.Mode != ModeQuick || item.AllowType != "" || item.AllowValue != "" {
		t.Fatalf("quick item = %+v", item)
	}
	if item.Hostname == "" || !isQuickPlaceholder(item.Hostname) {
		t.Fatalf("quick hostname = %q, want internal placeholder", item.Hostname)
	}

	_, err = useCase.Create(context.Background(), CreateInput{
		Name: "Invalid", Mode: ModeQuick, Hostname: "share.example.com", OriginURL: "http://127.0.0.1:3000",
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "hostname" {
		t.Fatalf("quick hostname error = %v, want hostname validation", err)
	}
}

func TestUseCasePrivateRequiresMatchingPrivateOrigin(t *testing.T) {
	repo := newMemoryRepository()
	useCase := NewUseCase(repo, "workspace")
	item, err := useCase.Create(context.Background(), CreateInput{
		Name: "LAN app", Mode: ModePrivate, Hostname: "192.168.1.20", OriginURL: "http://192.168.1.20:8080",
		AllowType: AllowEmail, AllowValue: "user@example.com",
	})
	if err != nil {
		t.Fatalf("create private: %v", err)
	}
	if item.Mode != ModePrivate || item.Hostname != "192.168.1.20" {
		t.Fatalf("private item = %+v", item)
	}

	for _, input := range []CreateInput{
		{Name: "public", Mode: ModePrivate, Hostname: "8.8.8.8", OriginURL: "http://8.8.8.8:80", AllowType: AllowEmail, AllowValue: "user@example.com"},
		{Name: "mismatch", Mode: ModePrivate, Hostname: "192.168.1.20", OriginURL: "http://192.168.1.21:8080", AllowType: AllowEmail, AllowValue: "user@example.com"},
	} {
		if _, err := useCase.Create(context.Background(), input); err == nil {
			t.Fatalf("create %q unexpectedly succeeded", input.Name)
		}
	}
}

func TestUseCaseUpdateBlocksDeployingService(t *testing.T) {
	repo := newMemoryRepository()
	useCase := NewUseCase(repo, "workspace")
	item, err := useCase.Create(context.Background(), CreateInput{
		Name: "Demo", Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: AllowEmailDomain, AllowValue: "example.com",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	item.State = StateDeploying
	repo.items[item.ID] = item
	newName := "Changed"
	_, err = useCase.Update(context.Background(), item.ID, UpdateInput{Name: &newName})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("update error = %v, want conflict", err)
	}
}

func TestUseCaseUpdateBlocksActiveService(t *testing.T) {
	repo := newMemoryRepository()
	useCase := NewUseCase(repo, "workspace")
	item, err := useCase.Create(context.Background(), CreateInput{
		Name: "Demo", Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: AllowEmail, AllowValue: "user@example.com",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	item.State = StateActive
	repo.items[item.ID] = item
	newName := "Changed"
	if _, err := useCase.Update(context.Background(), item.ID, UpdateInput{Name: &newName}); !errors.Is(err, ErrConflict) {
		t.Fatalf("update error = %v, want conflict", err)
	}
}

func TestUseCaseDeleteBlocksManagedService(t *testing.T) {
	repo := newMemoryRepository()
	useCase := NewUseCase(repo, "workspace")
	item, err := useCase.Create(context.Background(), CreateInput{
		Name: "Demo", Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: AllowEmail, AllowValue: "user@example.com",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	item.State = StateActive
	item.TunnelID = "tun_1"
	repo.items[item.ID] = item

	if err := useCase.Delete(context.Background(), item.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete error = %v, want conflict", err)
	}
	if _, err := useCase.Get(context.Background(), item.ID); err != nil {
		t.Fatalf("managed service was deleted: %v", err)
	}
}

func TestValidHostname(t *testing.T) {
	for _, value := range []string{"example.com", "a-1.internal"} {
		if !validHostname(value) {
			t.Errorf("validHostname(%q) = false", value)
		}
	}
	for _, value := range []string{"", "-bad.example", "bad-.example", "127.0.0.1", "bad_underscore.example"} {
		if validHostname(value) {
			t.Errorf("validHostname(%q) = true", value)
		}
	}
}
