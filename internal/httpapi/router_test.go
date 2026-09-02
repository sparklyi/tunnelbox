package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/service"
)

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

func TestRouterRequiresBearerTokenAndReturnsRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := NewRouter(Dependencies{
		Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, AdminToken: "secret",
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

func TestRouterCreatesServiceWithBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	services := &fakeServiceActions{}
	router, err := NewRouter(Dependencies{Services: services, Operations: fakeOperationReader{}, AdminToken: "secret"})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	body := `{"name":"Demo","hostname":"app.example.com","origin_url":"http://127.0.0.1:8080","allow_type":"email","allow_value":"user@example.com"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/services", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer secret")
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
