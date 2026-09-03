package provision

import (
	"context"
	"database/sql"
	"errors"
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
	calls         *[]string
	routes        *[]RouteSpec
	privateRoutes *[]PrivateRouteSpec
}

func (p recordingTunnel) EnsureTunnel(context.Context, TunnelSpec) (RemoteTunnel, error) {
	*p.calls = append(*p.calls, "tunnel")
	return RemoteTunnel{ID: "tun_1", ConnectorToken: "token_1"}, nil
}

func (p recordingTunnel) ApplyWebRoute(_ context.Context, spec RouteSpec) error {
	*p.calls = append(*p.calls, "route")
	if p.routes != nil {
		*p.routes = append(*p.routes, spec)
	}
	return nil
}

func (p recordingTunnel) EnsurePrivateRoute(_ context.Context, spec PrivateRouteSpec) (RemoteRef, error) {
	*p.calls = append(*p.calls, "private_route")
	if p.privateRoutes != nil {
		*p.privateRoutes = append(*p.privateRoutes, spec)
	}
	return RemoteRef{ID: "route_1"}, nil
}

type recordingAccess struct {
	calls        *[]string
	policyIDs    *[]string
	applications *[]AccessApplicationSpec
}

func (p recordingAccess) EnsureApplication(_ context.Context, spec AccessApplicationSpec) (RemoteRef, error) {
	*p.calls = append(*p.calls, "application")
	if p.policyIDs != nil {
		*p.policyIDs = append(*p.policyIDs, spec.PolicyID)
	}
	if p.applications != nil {
		*p.applications = append(*p.applications, spec)
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

type cleanupTunnel struct {
	recordingTunnel
	privateRouteError error
	tunnelError       error
}

func (p cleanupTunnel) DeletePrivateRoute(context.Context, string) error {
	*p.calls = append(*p.calls, "private_route_delete")
	return p.privateRouteError
}

func (p cleanupTunnel) DeleteTunnel(context.Context, string) error {
	*p.calls = append(*p.calls, "tunnel_delete")
	return p.tunnelError
}

type cleanupAccess struct {
	recordingAccess
	policyError      error
	applicationError error
}

func (p cleanupAccess) DeletePolicy(context.Context, string, string) error {
	*p.calls = append(*p.calls, "policy_delete")
	return p.policyError
}

func (p cleanupAccess) DeleteApplication(context.Context, string) error {
	*p.calls = append(*p.calls, "application_delete")
	return p.applicationError
}

type cleanupDNS struct {
	recordingDNS
	deleteError error
}

func (p cleanupDNS) DeleteCNAME(context.Context, string) error {
	*p.calls = append(*p.calls, "dns_delete")
	return p.deleteError
}

type recordingConnector struct {
	calls     *[]string
	specs     *[]ConnectorSpec
	status    ConnectorStatus
	stopError error
}

func (p recordingConnector) EnsureRunning(_ context.Context, spec ConnectorSpec) error {
	*p.calls = append(*p.calls, "connector")
	if p.specs != nil {
		*p.specs = append(*p.specs, spec)
	}
	return nil
}

func (p recordingConnector) Reload(context.Context, string) error { return nil }

func (p recordingConnector) Status(context.Context, string) (ConnectorStatus, error) {
	*p.calls = append(*p.calls, "health")
	if p.status.Running || p.status.Healthy || p.status.URL != "" || p.status.Message != "" {
		return p.status, nil
	}
	return ConnectorStatus{Running: true, Healthy: true}, nil
}

func (p recordingConnector) Stop(context.Context, string) error {
	*p.calls = append(*p.calls, "stop")
	return p.stopError
}

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
	deployer, err := NewDeployer(services, operations, recordingTunnel{calls: &calls}, recordingAccess{calls: &calls, policyIDs: &policyIDs}, recordingDNS{&calls}, recordingConnector{calls: &calls}, recordingOrigin{&calls})
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

func TestDeployerQuickSkipsCloudflareAndPersistsTemporaryURL(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Preview", Mode: service.ModeQuick, OriginURL: "http://127.0.0.1:3000",
	})
	defer db.Close()

	var calls []string
	var specs []ConnectorSpec
	connector := recordingConnector{calls: &calls, specs: &specs, status: ConnectorStatus{
		Running: true, Healthy: true, URL: "https://preview.trycloudflare.com",
	}}
	deployer, err := NewDeployer(services, operations, nil, nil, nil, connector, recordingOrigin{&calls})
	if err != nil {
		t.Fatalf("new quick deployer: %v", err)
	}
	op, err := deployer.Deploy(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start quick deploy: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusSucceeded {
		t.Fatalf("quick operation = %+v", current)
	}
	if !reflect.DeepEqual(calls, []string{"origin", "connector", "health", "health"}) {
		t.Fatalf("quick calls = %v", calls)
	}
	if len(specs) != 1 || !specs[0].Quick || specs[0].OriginURL != item.OriginURL || specs[0].TunnelID != "" {
		t.Fatalf("quick connector spec = %+v", specs)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load quick service: %v", err)
	}
	if loaded.State != service.StateActive || loaded.PublicURL != "https://preview.trycloudflare.com" || loaded.TunnelID != "" {
		t.Fatalf("quick service after deploy = %+v", loaded)
	}
}

func TestDeployerStopsQuickServiceAndClearsTemporaryURL(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Preview", Mode: service.ModeQuick, OriginURL: "http://127.0.0.1:3000",
	})
	defer db.Close()
	item.State = service.StateActive
	item.PublicURL = "https://preview.trycloudflare.com"
	if err := services.SetRemoteRefs(context.Background(), item.ID, item.RemoteRefs); err != nil {
		t.Fatalf("set remote refs: %v", err)
	}
	if err := services.SetState(context.Background(), item.ID, service.StateActive); err != nil {
		t.Fatalf("set active state: %v", err)
	}

	var calls []string
	deployer, err := NewDeployer(services, operations, nil, nil, nil, recordingConnector{calls: &calls}, nil)
	if err != nil {
		t.Fatalf("new stop deployer: %v", err)
	}
	op, err := deployer.Stop(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start stop: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusSucceeded || current.Kind != "stop" {
		t.Fatalf("stop operation = %+v", current)
	}
	if !reflect.DeepEqual(calls, []string{"stop"}) {
		t.Fatalf("stop calls = %v", calls)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load stopped service: %v", err)
	}
	if loaded.State != service.StateStopped || loaded.PublicURL != "" {
		t.Fatalf("quick service after stop = %+v", loaded)
	}
}

func TestDeployerStopsManagedServiceAndKeepsRemoteReferences(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Public app", Mode: service.ModePublic, Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: service.AllowEmail, AllowValue: "user@example.com",
	})
	defer db.Close()
	item.State = service.StateActive
	item.RemoteRefs = service.RemoteRefs{TunnelID: "tun_1", DNSRecordID: "dns_1", AccessApplicationID: "app_1", AccessPolicyID: "policy_1"}
	if err := services.SetRemoteRefs(context.Background(), item.ID, item.RemoteRefs); err != nil {
		t.Fatalf("set remote refs: %v", err)
	}
	if err := services.SetState(context.Background(), item.ID, service.StateActive); err != nil {
		t.Fatalf("set active state: %v", err)
	}

	var calls []string
	deployer, err := NewDeployer(services, operations, nil, nil, nil, recordingConnector{calls: &calls}, nil)
	if err != nil {
		t.Fatalf("new stop deployer: %v", err)
	}
	op, err := deployer.Stop(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start stop: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusSucceeded {
		t.Fatalf("stop operation = %+v", current)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load stopped service: %v", err)
	}
	if loaded.State != service.StateStopped || loaded.RemoteRefs != item.RemoteRefs {
		t.Fatalf("managed service after stop = %+v, want refs %+v", loaded, item.RemoteRefs)
	}
}

func TestDeployerDeletesManagedServiceAndKeepsOperationHistory(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Public app", Mode: service.ModePublic, Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: service.AllowEmail, AllowValue: "user@example.com",
	})
	defer db.Close()
	refs := service.RemoteRefs{TunnelID: "tun_1", DNSRecordID: "dns_1", AccessApplicationID: "app_1", AccessPolicyID: "policy_1"}
	if err := services.SetRemoteRefs(context.Background(), item.ID, refs); err != nil {
		t.Fatalf("set remote refs: %v", err)
	}
	if err := services.SetState(context.Background(), item.ID, service.StateStopped); err != nil {
		t.Fatalf("set stopped state: %v", err)
	}

	var calls []string
	deployer, err := NewDeployer(services, operations,
		cleanupTunnel{recordingTunnel: recordingTunnel{calls: &calls}},
		cleanupAccess{recordingAccess: recordingAccess{calls: &calls}},
		cleanupDNS{recordingDNS: recordingDNS{calls: &calls}},
		recordingConnector{calls: &calls}, nil)
	if err != nil {
		t.Fatalf("new delete deployer: %v", err)
	}
	op, err := deployer.Delete(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start delete: %v", err)
	}
	if op.ID == "" || op.Kind != "delete" {
		t.Fatalf("delete operation = %+v", op)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusSucceeded {
		t.Fatalf("delete operation = %+v", current)
	}
	wantCalls := []string{"stop", "dns_delete", "policy_delete", "application_delete", "tunnel_delete"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("delete calls = %v, want %v", calls, wantCalls)
	}
	if _, err := services.Get(context.Background(), item.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("deleted service lookup = %v, want not found", err)
	}
	history, err := operations.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatalf("get delete operation history: %v", err)
	}
	if history.Status != operation.StatusSucceeded || history.Kind != "delete" || history.ServiceID != item.ID {
		t.Fatalf("delete operation history = %+v", history)
	}
}

func TestDeployerDeletesDraftLocallyWithoutStartingOperation(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{Name: "Draft", Mode: service.ModeQuick, OriginURL: "http://127.0.0.1:3000"})
	defer db.Close()
	var calls []string
	deployer, err := NewDeployer(services, operations, nil, nil, nil, recordingConnector{calls: &calls}, nil)
	if err != nil {
		t.Fatalf("new delete deployer: %v", err)
	}
	op, err := deployer.Delete(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("delete draft: %v", err)
	}
	if op.ID != "" {
		t.Fatalf("draft delete unexpectedly started operation: %+v", op)
	}
	if _, err := services.Get(context.Background(), item.ID); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("draft lookup after delete = %v, want not found", err)
	}
	if len(calls) != 0 {
		t.Fatalf("draft delete started connector calls: %v", calls)
	}
}

func TestDeployerDeleteFailureKeepsRemainingReferences(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Public app", Mode: service.ModePublic, Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: service.AllowEmail, AllowValue: "user@example.com",
	})
	defer db.Close()
	refs := service.RemoteRefs{TunnelID: "tun_1", DNSRecordID: "dns_1", AccessApplicationID: "app_1", AccessPolicyID: "policy_1"}
	if err := services.SetRemoteRefs(context.Background(), item.ID, refs); err != nil {
		t.Fatalf("set remote refs: %v", err)
	}
	if err := services.SetState(context.Background(), item.ID, service.StateStopped); err != nil {
		t.Fatalf("set stopped state: %v", err)
	}
	var calls []string
	deployer, err := NewDeployer(services, operations,
		cleanupTunnel{recordingTunnel: recordingTunnel{calls: &calls}},
		cleanupAccess{recordingAccess: recordingAccess{calls: &calls}},
		cleanupDNS{recordingDNS: recordingDNS{calls: &calls}, deleteError: errors.New("dns unavailable")},
		recordingConnector{calls: &calls}, nil)
	if err != nil {
		t.Fatalf("new delete deployer: %v", err)
	}
	op, err := deployer.Delete(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start delete: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusFailed || current.ErrorCode != "dns_delete_failed" {
		t.Fatalf("failed delete operation = %+v", current)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load service after failed delete: %v", err)
	}
	if loaded.State != service.StateError || loaded.RemoteRefs != refs {
		t.Fatalf("service after failed delete = %+v, want state error and refs %+v", loaded, refs)
	}
	if !reflect.DeepEqual(calls, []string{"stop", "dns_delete"}) {
		t.Fatalf("failed delete calls = %v", calls)
	}
}

func TestDeployerStopFailureMarksServiceError(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Preview", Mode: service.ModeQuick, OriginURL: "http://127.0.0.1:3000",
	})
	defer db.Close()
	if err := services.SetState(context.Background(), item.ID, service.StateActive); err != nil {
		t.Fatalf("set active state: %v", err)
	}
	var calls []string
	deployer, err := NewDeployer(services, operations, nil, nil, nil, recordingConnector{calls: &calls, stopError: errors.New("stop failed")}, nil)
	if err != nil {
		t.Fatalf("new stop deployer: %v", err)
	}
	op, err := deployer.Stop(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start stop: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusFailed || current.ErrorCode != "connector_stop_failed" {
		t.Fatalf("failed stop operation = %+v", current)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load failed service: %v", err)
	}
	if loaded.State != service.StateError {
		t.Fatalf("service state after failed stop = %s, want error", loaded.State)
	}
}

func TestDeployerPrivateUsesWarpRouteAndSkipsDNS(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "LAN app", Mode: service.ModePrivate, Hostname: "192.168.1.20", OriginURL: "http://192.168.1.20:8080",
		AllowType: service.AllowEmail, AllowValue: "user@example.com",
	})
	defer db.Close()

	var calls []string
	var routes []RouteSpec
	var privateRoutes []PrivateRouteSpec
	var applications []AccessApplicationSpec
	tunnel := recordingTunnel{calls: &calls, routes: &routes, privateRoutes: &privateRoutes}
	access := recordingAccess{calls: &calls, applications: &applications}
	deployer, err := NewDeployer(services, operations, tunnel, access, nil, recordingConnector{calls: &calls}, recordingOrigin{&calls})
	if err != nil {
		t.Fatalf("new private deployer: %v", err)
	}
	op, err := deployer.Deploy(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start private deploy: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusSucceeded {
		t.Fatalf("private operation = %+v", current)
	}
	expected := []string{"origin", "tunnel", "route", "private_route", "connector", "health", "application", "policy", "application"}
	if !reflect.DeepEqual(calls, expected) {
		t.Fatalf("private calls = %v, want %v", calls, expected)
	}
	if len(routes) != 1 || !routes[0].Private || routes[0].Hostname != "" || routes[0].OriginURL != "" {
		t.Fatalf("private route spec = %+v", routes)
	}
	if len(privateRoutes) != 1 || privateRoutes[0].Network != "192.168.1.20/32" || privateRoutes[0].TunnelID != "tun_1" {
		t.Fatalf("private network route = %+v", privateRoutes)
	}
	if len(applications) != 2 || !applications[0].Private || applications[0].Domain != "192.168.1.20:8080" || applications[1].PolicyID != "policy_1" {
		t.Fatalf("private applications = %+v", applications)
	}
	loaded, err := services.Get(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("load private service: %v", err)
	}
	if loaded.State != service.StateActive || loaded.PrivateRouteID != "route_1" || loaded.DNSRecordID != "" || loaded.PublicURL != "" {
		t.Fatalf("private service after deploy = %+v", loaded)
	}
}

func TestDeployerPublicRequiresDNSAdapter(t *testing.T) {
	db, services, operations, item := newDeploymentFixture(t, service.CreateInput{
		Name: "Public app", Mode: service.ModePublic, Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080",
		AllowType: service.AllowEmail, AllowValue: "user@example.com",
	})
	defer db.Close()

	var calls []string
	deployer, err := NewDeployer(services, operations, recordingTunnel{calls: &calls}, recordingAccess{calls: &calls}, nil, recordingConnector{calls: &calls}, nil)
	if err != nil {
		t.Fatalf("new public deployer: %v", err)
	}
	op, err := deployer.Deploy(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("start public deploy: %v", err)
	}
	current := waitForOperation(t, operations, op.ID)
	if current.Status != operation.StatusFailed || current.ErrorCode != "cloudflare_not_configured" {
		t.Fatalf("public operation = %+v, want missing Cloudflare adapter", current)
	}
	if len(calls) != 0 {
		t.Fatalf("adapters were called before dependency check: %v", calls)
	}
}

func newDeploymentFixture(t *testing.T, input service.CreateInput) (*sql.DB, *service.UseCase, *operation.Manager, service.Service) {
	t.Helper()
	db, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "tunnelbox.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	store := sqlite.NewStore(db)
	if err := store.EnsureWorkspace(context.Background(), "default", "Default"); err != nil {
		db.Close()
		t.Fatalf("workspace: %v", err)
	}
	services := service.NewUseCase(store.Services(), "default")
	item, err := services.Create(context.Background(), input)
	if err != nil {
		db.Close()
		t.Fatalf("create service: %v", err)
	}
	return db, services, operation.NewManager(store.Operations()), item
}

func waitForOperation(t *testing.T, operations *operation.Manager, id string) operation.Operation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		current, err := operations.Get(context.Background(), id)
		if err != nil {
			t.Fatalf("get operation: %v", err)
		}
		if current.Status == operation.StatusSucceeded || current.Status == operation.StatusFailed || current.Status == operation.StatusUnknown {
			return current
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not finish: %+v", current)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestDeployerResumeSupportsKnownOperations(t *testing.T) {
	deployer := &Deployer{}
	if task := deployer.Resume(operation.Operation{Kind: "unknown", ServiceID: "svc_1"}); task != nil {
		t.Fatal("resume returned a task for an unsupported operation kind")
	}
	if task := deployer.Resume(operation.Operation{Kind: "stop", ServiceID: "svc_1"}); task == nil {
		t.Fatal("resume did not return a task for a stop operation")
	}
	if task := deployer.Resume(operation.Operation{Kind: "delete", ServiceID: "svc_1"}); task == nil {
		t.Fatal("resume did not return a task for a delete operation")
	}
}
