package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sparklyi/tunnelbox/internal/auth"
	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/provision"
	"github.com/sparklyi/tunnelbox/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type testAuthRepository struct{ sessions map[string]time.Time }

func (r *testAuthRepository) PasswordHash(context.Context) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
}
func (r *testAuthRepository) SavePasswordHash(context.Context, []byte) error { return nil }
func (r *testAuthRepository) CreateSession(_ context.Context, token string, expires time.Time) error {
	r.sessions[token] = expires
	return nil
}
func (r *testAuthRepository) SessionValid(_ context.Context, token string, now time.Time) (bool, error) {
	expires, ok := r.sessions[token]
	return ok && expires.After(now), nil
}
func (r *testAuthRepository) DeleteSession(_ context.Context, token string) error {
	delete(r.sessions, token)
	return nil
}
func testAuth(t *testing.T) *auth.Manager {
	return auth.NewManager(&testAuthRepository{sessions: map[string]time.Time{}})
}
func addTestCookie(t *testing.T, req *http.Request, manager *auth.Manager) {
	token, err := manager.Login(context.Background(), "password123")
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
}

type fakeServiceActions struct {
	items []service.Service
}

func (f *fakeServiceActions) List(context.Context) ([]service.Service, error) { return f.items, nil }

func (f *fakeServiceActions) Get(_ context.Context, id string) (service.Service, error) {
	for _, item := range f.items {
		if item.ID == id {
			return item, nil
		}
	}
	return service.Service{}, service.ErrNotFound
}

func (f *fakeServiceActions) Create(_ context.Context, input service.CreateInput) (service.Service, error) {
	item := service.Service{ID: "svc_test", Name: input.Name, Hostname: input.Hostname, OriginURL: input.OriginURL,
		AllowType: input.AllowType, AllowValue: input.AllowValue, State: service.StateDraft,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}
	f.items = append(f.items, item)
	return item, nil
}

func (f *fakeServiceActions) Update(context.Context, string, service.UpdateInput) (service.Service, error) {
	return service.Service{}, errors.New("not used")
}

func (f *fakeServiceActions) Delete(context.Context, string) error { return nil }

type fakeOperationReader struct{}

func (fakeOperationReader) Get(context.Context, string) (operation.Operation, error) {
	return operation.Operation{ID: "op_test", ServiceID: "svc_test", Kind: "deploy", Status: operation.StatusPending,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}, nil
}

type fakeServiceStopper struct {
	called bool
}

func (f *fakeServiceStopper) Stop(context.Context, string) (operation.Operation, error) {
	f.called = true
	return operation.Operation{ID: "op_stop", ServiceID: "svc_test", Kind: "stop", Status: operation.StatusPending,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}, nil
}

type fakeServiceDeleter struct {
	called bool
}

func (f *fakeServiceDeleter) Delete(context.Context, string) (operation.Operation, error) {
	f.called = true
	return operation.Operation{ID: "op_delete", ServiceID: "svc_test", Kind: "delete", Status: operation.StatusPending,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}, nil
}

type fakeCloudflareIntegration struct{}

func (fakeCloudflareIntegration) Configure(context.Context, provision.CloudflareConfigureInput) (provision.CloudflareIntegrationStatus, error) {
	return provision.CloudflareIntegrationStatus{Configured: true, AccountID: "acct", ZoneID: "zone", TokenID: "tok_1", TokenState: "active"}, nil
}

func (fakeCloudflareIntegration) Status(context.Context) (provision.CloudflareIntegrationStatus, error) {
	return provision.CloudflareIntegrationStatus{Configured: true, AccountID: "acct", ZoneID: "zone"}, nil
}

func (fakeCloudflareIntegration) Zones(context.Context) ([]provision.Zone, error) {
	return []provision.Zone{{ID: "zone", Name: "example.com"}}, nil
}

func TestRouterRequiresBearerTokenAndReturnsRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := NewRouter(Dependencies{
		Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Auth: testAuth(t),
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body["code"] != "unauthorized" {
		t.Fatalf("error body = %v", body)
	}
}

func TestRouterServesConsoleShellBeforeBearerAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html>console</html>"), 0o600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	router, err := NewRouter(Dependencies{Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Auth: testAuth(t), WebDir: webDir})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "console") {
		t.Fatalf("console response = %d %s", response.Code, response.Body.String())
	}
}

func TestRouterCreatesServiceWithBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	services := &fakeServiceActions{}
	authentication := testAuth(t)
	router, err := NewRouter(Dependencies{Services: services, Operations: fakeOperationReader{}, Auth: authentication})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	body := `{"name":"Demo","hostname":"app.example.com","origin_url":"http://127.0.0.1:8080","allow_type":"email","allow_value":"user@example.com"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/services", strings.NewReader(body))
	addTestCookie(t, request, authentication)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(services.items) != 1 || services.items[0].ID != "svc_test" {
		t.Fatalf("services = %+v", services.items)
	}
}

func TestRouterStopsServiceWithBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stopper := &fakeServiceStopper{}
	authentication := testAuth(t)
	router, err := NewRouter(Dependencies{Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Stopper: stopper, Auth: authentication})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/services/svc_test/stop", nil)
	addTestCookie(t, request, authentication)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !stopper.called || !strings.Contains(response.Body.String(), `"kind":"stop"`) {
		t.Fatalf("stop response = %s, called = %v", response.Body.String(), stopper.called)
	}
}

func TestRouterDeletesServiceWithBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deleter := &fakeServiceDeleter{}
	authentication := testAuth(t)
	router, err := NewRouter(Dependencies{Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Deleter: deleter, Auth: authentication})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/services/svc_test", nil)
	addTestCookie(t, request, authentication)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !deleter.called || !strings.Contains(response.Body.String(), `"kind":"delete"`) {
		t.Fatalf("delete response = %s, called = %v", response.Body.String(), deleter.called)
	}
}

func TestRouterCloudflareIntegrationEndpointsDoNotReturnToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authentication := testAuth(t)
	router, err := NewRouter(Dependencies{Services: &fakeServiceActions{}, Operations: fakeOperationReader{},
		Cloudflare: fakeCloudflareIntegration{}, Auth: authentication})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/integrations/cloudflare", strings.NewReader(`{"account_id":"acct","zone_id":"zone","token":"super-secret"}`))
	addTestCookie(t, request, authentication)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "super-secret") {
		t.Fatalf("token leaked in response: %s", response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/zones", nil)
	addTestCookie(t, request, authentication)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "example.com") {
		t.Fatalf("zones response = %d %s", response.Code, response.Body.String())
	}
}
