package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

type testAuthRepository struct {
	hash     []byte
	sessions map[string]time.Time
}

func (r *testAuthRepository) PasswordHash(context.Context) ([]byte, error) { return r.hash, nil }
func (r *testAuthRepository) SavePasswordHash(_ context.Context, hash []byte) error {
	if len(r.hash) > 0 {
		return auth.ErrAlreadySetup
	}
	r.hash = hash
	return nil
}
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
func (r *testAuthRepository) DeleteExpiredSessions(_ context.Context, now time.Time) error {
	for token, expires := range r.sessions {
		if !expires.After(now) {
			delete(r.sessions, token)
		}
	}
	return nil
}
func testAuth(t *testing.T) *auth.Manager {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	return auth.NewManager(&testAuthRepository{hash: hash, sessions: map[string]time.Time{}})
}

func TestRouterUsesConsistentSecureSessionCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("setup", func(t *testing.T) {
		manager := auth.NewManager(&testAuthRepository{sessions: map[string]time.Time{}})
		router := newTestRouter(t, manager, true)
		response := performJSONRequest(router, http.MethodPost, "/api/v1/auth/setup", `{"password":"password123"}`, nil)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		assertSessionCookie(t, response, true, false)
	})

	t.Run("login", func(t *testing.T) {
		manager := testAuth(t)
		router := newTestRouter(t, manager, true)
		response := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login", `{"password":"password123"}`, nil)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		assertSessionCookie(t, response, true, false)
	})

	t.Run("logout", func(t *testing.T) {
		manager := testAuth(t)
		router := newTestRouter(t, manager, true)
		response := performJSONRequest(router, http.MethodPost, "/api/v1/auth/logout", "", &http.Cookie{Name: auth.SessionCookie, Value: "session"})
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		assertSessionCookie(t, response, true, true)
	})
}

func TestRouterRejectsInvalidJSONBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(t, testAuth(t), false)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "unknown field", body: `{"password":"password123","extra":true}`, status: http.StatusBadRequest, code: "invalid_json"},
		{name: "extra object", body: `{"password":"password123"}{}`, status: http.StatusBadRequest, code: "invalid_json"},
		{name: "too large", body: `{"password":"` + strings.Repeat("x", int(maxJSONBodyBytes)) + `"}`, status: http.StatusRequestEntityTooLarge, code: "request_too_large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login", tt.body, nil)
			if response.Code != tt.status || !strings.Contains(response.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestRouterRateLimitsAuthenticationByDirectPeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newTestRouter(t, testAuth(t), false)
	for attempt := 1; attempt <= 6; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
		request.RemoteAddr = "198.51.100.10:1234"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", attempt))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if attempt <= 5 && response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, body = %s", attempt, response.Code, response.Body.String())
		}
		if attempt == 6 && (response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), `"code":"rate_limited"`)) {
			t.Fatalf("rate-limited response = %d %s", response.Code, response.Body.String())
		}
	}
}

func newTestRouter(t *testing.T, manager *auth.Manager, secure bool) http.Handler {
	t.Helper()
	router, err := NewRouter(Dependencies{
		Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Auth: manager, SecureCookies: secure,
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}

func performJSONRequest(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertSessionCookie(t *testing.T, response *httptest.ResponseRecorder, secure, deleted bool) {
	t.Helper()
	var session *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == auth.SessionCookie {
			session = cookie
			break
		}
	}
	if session == nil {
		t.Fatal("session cookie not set")
	}
	if session.Path != "/" || !session.HttpOnly || session.Secure != secure || session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie attributes = %+v", session)
	}
	if deleted != (session.MaxAge < 0) {
		t.Fatalf("cookie MaxAge = %d, deleted = %v", session.MaxAge, deleted)
	}
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

func TestRouterValidatesAndPropagatesRequestID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "safe client id", value: "client.ID_123-abc", valid: true},
		{name: "space", value: "bad request", valid: false},
		{name: "quote", value: `bad"request`, valid: false},
		{name: "control", value: "bad\x01request", valid: false},
		{name: "unicode", value: "request-编号", valid: false},
		{name: "too long", value: strings.Repeat("a", 97), valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			router, err := NewRouter(Dependencies{
				Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Auth: testAuth(t), Logger: logger,
			})
			if err != nil {
				t.Fatalf("new router: %v", err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
			request.Header.Set("X-Request-ID", tt.value)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			responseID := response.Header().Get("X-Request-ID")
			if !validRequestID(responseID) {
				t.Fatalf("response request id = %q", responseID)
			}
			if tt.valid && responseID != tt.value {
				t.Fatalf("response request id = %q, want %q", responseID, tt.value)
			}
			if !tt.valid && responseID == tt.value {
				t.Fatalf("unsafe request id was accepted: %q", responseID)
			}

			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			var entry map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
				t.Fatalf("decode log: %v", err)
			}
			if body["request_id"] != responseID || entry["request_id"] != responseID {
				t.Fatalf("request ids differ: header=%q body=%v log=%v", responseID, body["request_id"], entry["request_id"])
			}
		})
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
