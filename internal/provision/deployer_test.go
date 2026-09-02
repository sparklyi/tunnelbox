package provision

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
	"github.com/sparklyi/tunnelbox/internal/store/sqlite"
)

type recordingOrigin struct {
	calls *[]string
}

func (o recordingOrigin) Check(context.Context, string) error {
	*o.calls = append(*o.calls, "origin")
	return nil
}

type recordingTunnel struct {
	calls *[]string
}

func (p recordingTunnel) EnsureTunnel(context.Context, TunnelSpec) (RemoteTunnel, error) {
	*p.calls = append(*p.calls, "tunnel")
	return RemoteTunnel{ID: "tun_1", ConnectorToken: "token_1"}, nil
}

func (p recordingTunnel) ApplyWebRoute(context.Context, RouteSpec) error {
	*p.calls = append(*p.calls, "route")
	return nil
}

type recordingAccess struct {
	calls     *[]string
	policyIDs *[]string
}

func (p recordingAccess) EnsureApplication(_ context.Context, spec AccessApplicationSpec) (RemoteRef, error) {
	*p.calls = append(*p.calls, "application")
	if p.policyIDs != nil {
		*p.policyIDs = append(*p.policyIDs, spec.PolicyID)
	}
	return RemoteRef{ID: "app_1"}, nil
}

func (p recordingAccess) EnsurePolicy(context.Context, AccessPolicySpec) (RemoteRef, error) {
	*p.calls = append(*p.calls, "policy")
	return RemoteRef{ID: "policy_1"}, nil
}

type recordingDNS struct {
	calls *[]string
}

func (p recordingDNS) EnsureCNAME(context.Context, CNAMESpec) (RemoteRef, error) {
	*p.calls = append(*p.calls, "dns")
	return RemoteRef{ID: "dns_1"}, nil
}

type recordingConnector struct {
	calls *[]string
}

func (p recordingConnector) EnsureRunning(context.Context, ConnectorSpec) error {
	*p.calls = append(*p.calls, "connector")
	return nil
}

func (p recordingConnector) Reload(context.Context, string) error { return nil }

func (p recordingConnector) Status(context.Context, string) (ConnectorStatus, error) {
	*p.calls = append(*p.calls, "health")
	return ConnectorStatus{Running: true, Healthy: true}, nil
}

func (p recordingConnector) Stop(context.Context, string) error { return nil }

func TestDeployerPersistsReferencesAndAppliesSafeOrder(t *testing.T) {
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "tunnelbox.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)
	if err := store.EnsureWorkspace(context.Background(), "default", "Default"); err != nil {
		t.Fatalf("workspace: %v", err)
	}
	services := service.NewUseCase(store.Services(), "default")
	item, err := services.Create(context.Background(), service.CreateInput{
		Name: "Demo", Hostname: "demo.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: service.AllowEmail, AllowValue: "user@example.com",
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	var calls []string
	var policyIDs []string
	operations := operation.NewManager(store.Operations())
	deployer, err := NewDeployer(services, operations, recordingTunnel{&calls}, recordingAccess{calls: &calls, policyIDs: &policyIDs}, recordingDNS{&calls}, recordingConnector{&calls}, recordingOrigin{&calls})
	if err != nil {
		t.Fatalf("new deployer: %v", err)
	}
	op, err := deployer.Deploy(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		current, getErr := operations.Get(context.Background(), op.ID)
		if getErr != nil {
			t.Fatalf("get operation: %v", getErr)
		}
		if current.Status == operation.StatusSucceeded {
			break
		}
		if current.Status == operation.StatusFailed || current.Status == operation.StatusUnknown {
			t.Fatalf("operation ended unexpectedly: %+v", current)
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not finish: %+v", current)
		}
		time.Sleep(5 * time.Millisecond)
	}

	expected := []string{"origin", "tunnel", "route", "connector", "health", "application", "policy", "application", "dns"}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("calls = %v, want %v", calls, expected)
	}
	if !reflect.DeepEqual(policyIDs, []string{"", "policy_1"}) {
		t.Fatalf("application policy ids = %v, want initial and attached policy", policyIDs)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load service: %v", err)
	}
	if loaded.State != service.StateActive || loaded.TunnelID != "tun_1" || loaded.AccessApplicationID != "app_1" || loaded.AccessPolicyID != "policy_1" || loaded.DNSRecordID != "dns_1" {
		t.Fatalf("service after deploy = %+v", loaded)
	}
}

func TestDeployerResumeRejectsUnsupportedOperation(t *testing.T) {
	deployer := &Deployer{}
	if task := deployer.Resume(operation.Operation{Kind: "delete", ServiceID: "svc_1"}); task != nil {
		t.Fatal("resume returned a task for an unsupported operation kind")
	}
}
