package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
)

func TestRepositoriesRoundTrip(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "tunnelbox.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()
	if err := store.EnsureWorkspace(ctx, "default", "Default"); err != nil {
		t.Fatalf("workspace: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	item := service.Service{ID: "svc_1", WorkspaceID: "default", Name: "Demo", Mode: service.ModePrivate, Hostname: "192.168.1.20",
		OriginURL: "http://192.168.1.20:8080", AllowType: service.AllowEmail, AllowValue: "user@example.com",
		State: service.StateDraft, CreatedAt: now, UpdatedAt: now}
	if err := store.Services().Create(ctx, item); err != nil {
		t.Fatalf("create service: %v", err)
	}
	loaded, err := store.Services().Get(ctx, "default", item.ID)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	if loaded.ID != item.ID || loaded.Mode != item.Mode || loaded.Hostname != item.Hostname || !loaded.CreatedAt.Equal(item.CreatedAt) {
		t.Fatalf("loaded service = %+v", loaded)
	}
	if err := store.Services().SetRemoteRefs(ctx, "default", item.ID, service.RemoteRefs{TunnelID: "tun_1", PrivateRouteID: "route_1", DNSRecordID: "dns_1", PublicURL: "https://preview.example"}); err != nil {
		t.Fatalf("refs: %v", err)
	}
	loaded, err = store.Services().Get(ctx, "default", item.ID)
	if err != nil || loaded.TunnelID != "tun_1" || loaded.PrivateRouteID != "route_1" || loaded.DNSRecordID != "dns_1" || loaded.PublicURL != "https://preview.example" {
		t.Fatalf("refs not persisted: item=%+v err=%v", loaded, err)
	}

	op := operation.Operation{ID: "op_1", ServiceID: item.ID, Kind: "deploy", Status: operation.StatusPending, CreatedAt: now, UpdatedAt: now}
	if err := store.Operations().Create(ctx, op); err != nil {
		t.Fatalf("create operation: %v", err)
	}
	active, err := store.Operations().FindActiveForService(ctx, item.ID)
	if err != nil || active.ID != op.ID {
		t.Fatalf("active operation = %+v err=%v", active, err)
	}
	if err := store.Operations().Update(ctx, operation.Operation{ID: op.ID, ServiceID: item.ID, Kind: op.Kind,
		Status: operation.StatusSucceeded, CurrentStep: "complete", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("update operation: %v", err)
	}
	if _, err := store.Operations().FindActiveForService(ctx, item.ID); !errors.Is(err, operation.ErrNotFound) {
		t.Fatalf("active after completion = %v, want not found", err)
	}
}

func TestDeletingServiceKeepsOperationHistory(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "tunnelbox.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := context.Background()
	if err := store.EnsureWorkspace(ctx, "default", "Default"); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	now := time.Now().UTC()
	item := service.Service{ID: "svc_delete", WorkspaceID: "default", Name: "Draft", Mode: service.ModeQuick,
		Hostname: "quick-svc_delete.invalid", OriginURL: "http://127.0.0.1:3000", State: service.StateDraft,
		CreatedAt: now, UpdatedAt: now}
	if err := store.Services().Create(ctx, item); err != nil {
		t.Fatalf("create service: %v", err)
	}
	op := operation.Operation{ID: "op_delete", ServiceID: item.ID, Kind: "delete", Status: operation.StatusSucceeded, CreatedAt: now, UpdatedAt: now}
	if err := store.Operations().Create(ctx, op); err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if err := store.Services().Delete(ctx, item.WorkspaceID, item.ID); err != nil {
		t.Fatalf("delete service: %v", err)
	}
	history, err := store.Operations().Get(ctx, op.ID)
	if err != nil {
		t.Fatalf("get operation history: %v", err)
	}
	if history.ServiceID != item.ID || history.Kind != "delete" {
		t.Fatalf("operation history = %+v", history)
	}
}
