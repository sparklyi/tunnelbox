package cloudflare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIntegrationStatusRequiresActiveToken(t *testing.T) {
	tests := []struct {
		name       string
		tokenState string
		statusCode int
		configured bool
		lastError  string
	}{
		{name: "active", tokenState: "active", statusCode: http.StatusOK, configured: true},
		{name: "inactive", tokenState: "inactive", statusCode: http.StatusOK},
		{name: "expired", tokenState: "expired", statusCode: http.StatusOK},
		{name: "verification failure", statusCode: http.StatusInternalServerError, lastError: "cloudflare token verification failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tt.statusCode != http.StatusOK {
					w.WriteHeader(tt.statusCode)
					_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"secret upstream detail"}]}`))
					return
				}
				writeEnvelope(w, map[string]any{"id": "token_id", "status": tt.tokenState})
			}))
			defer server.Close()

			client, err := New(Config{Token: "secret", AccountID: "acct", BaseURL: server.URL + "/"})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			integration := &Integration{
				client: client,
				status: IntegrationStatus{Configured: true, AccountID: "acct"},
			}
			status, err := integration.Status(context.Background())
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if status.Configured != tt.configured || status.TokenState != tt.tokenState || status.LastError != tt.lastError {
				t.Fatalf("status = %+v", status)
			}
			if tt.lastError != "" && status.LastError == "secret upstream detail" {
				t.Fatal("status exposed the upstream error")
			}
		})
	}
}
