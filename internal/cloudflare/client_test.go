package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/sparklyi/tunnelbox/internal/provision"
)

func TestClientUsesSafeTunnelAccessAndDNSFlow(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user/tokens/verify":
			writeEnvelope(w, map[string]any{"id": "tok_1", "status": "active"})
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acct/cfd_tunnel":
			writeEnvelope(w, []any{})
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acct/cfd_tunnel/tun_1/token":
			writeEnvelope(w, "connector-token")
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acct/cfd_tunnel":
			if body["name"] != "tunnelbox-svc_1" {
				t.Errorf("tunnel body = %v", body)
			}
			writeEnvelope(w, map[string]any{"id": "tun_1", "name": "tunnelbox-svc_1"})
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acct/cfd_tunnel/tun_1/configurations":
			config, ok := body["config"].(map[string]any)
			if !ok {
				t.Errorf("route body = %v", body)
			} else {
				ingress, _ := config["ingress"].([]any)
				if len(ingress) != 2 || ingress[1].(map[string]any)["service"] != "http_status:404" {
					t.Errorf("ingress = %v", ingress)
				}
			}
			writeEnvelope(w, map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acct/access/apps":
			writeEnvelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acct/access/apps":
			writeEnvelope(w, map[string]any{"id": "app_1"})
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acct/access/apps/app_1/policies":
			writeEnvelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acct/access/apps/app_1/policies":
			include, _ := body["include"].([]any)
			if len(include) != 1 {
				t.Errorf("policy include = %v", include)
			}
			writeEnvelope(w, map[string]any{"id": "pol_1"})
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acct/access/apps/app_1":
			policies, _ := body["policies"].([]any)
			if len(policies) != 1 {
				t.Errorf("attached policies = %v", body["policies"])
			}
			writeEnvelope(w, map[string]any{"id": "app_1"})
		case r.Method == http.MethodGet && r.URL.Path == "/zones/zone/dns_records":
			writeEnvelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.Path == "/zones/zone/dns_records":
			if body["type"] != "CNAME" || body["content"] != "tun_1.cfargotunnel.com" {
				t.Errorf("dns body = %v", body)
			}
			writeEnvelope(w, map[string]any{"id": "dns_1"})
		default:
			http.Error(w, `{"success":false}`, http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := New(Config{Token: "secret", AccountID: "acct", ZoneID: "zone", BaseURL: server.URL + "/"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	status, err := client.VerifyToken(context.Background())
	if err != nil || status.Status != "active" {
		t.Fatalf("verify = %+v, err=%v", status, err)
	}
	tunnel, err := client.EnsureTunnel(context.Background(), provision.TunnelSpec{Name: "tunnelbox-svc_1"})
	if err != nil {
		t.Fatalf("tunnel: %v", err)
	}
	if tunnel.ID != "tun_1" || tunnel.ConnectorToken != "connector-token" {
		t.Fatalf("tunnel = %+v", tunnel)
	}
	if err := client.ApplyWebRoute(context.Background(), provision.RouteSpec{TunnelID: tunnel.ID, Hostname: "app.example.com", OriginURL: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("route: %v", err)
	}
	app, err := client.EnsureApplication(context.Background(), provision.AccessApplicationSpec{Name: "Demo", Domain: "app.example.com"})
	if err != nil {
		t.Fatalf("app: %v", err)
	}
	policy, err := client.EnsurePolicy(context.Background(), provision.AccessPolicySpec{ApplicationID: app.ID, Name: "Demo Allow", AllowType: "email", AllowValue: "user@example.com"})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	if _, err := client.EnsureApplication(context.Background(), provision.AccessApplicationSpec{ID: app.ID, Name: "Demo", Domain: "app.example.com", PolicyID: policy.ID}); err != nil {
		t.Fatalf("attach policy: %v", err)
	}
	dnsRecord, err := client.EnsureCNAME(context.Background(), provision.CNAMESpec{Name: "app.example.com", Target: tunnel.ID + ".cfargotunnel.com", Proxied: true})
	if err != nil || dnsRecord.ID != "dns_1" {
		t.Fatalf("dns = %+v, err=%v", dnsRecord, err)
	}

	mu.Lock()
	joined := strings.Join(paths, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "POST /accounts/acct/cfd_tunnel") || !strings.Contains(joined, "PUT /accounts/acct/cfd_tunnel/tun_1/configurations") {
		t.Fatalf("unexpected paths:\n%s", joined)
	}
}

func writeEnvelope(w http.ResponseWriter, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "errors": []any{}, "result": result})
}
