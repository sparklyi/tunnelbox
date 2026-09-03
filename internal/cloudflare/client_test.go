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

func TestClientPrivateRouteAndAccessApplication(t *testing.T) {
	var routeBody map[string]any
	var appBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acct/teamnet/routes":
			writeEnvelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acct/teamnet/routes":
			routeBody = body
			writeEnvelope(w, map[string]any{"id": "route_1", "network": body["network"], "tunnel_id": body["tunnel_id"]})
		case r.Method == http.MethodPut && r.URL.Path == "/accounts/acct/cfd_tunnel/tun_1/configurations":
			config, ok := body["config"].(map[string]any)
			if !ok {
				t.Errorf("private route config = %v", body)
			} else if warp, ok := config["warp-routing"].(map[string]any); !ok || warp["enabled"] != true {
				t.Errorf("warp routing config = %v", config)
			}
			writeEnvelope(w, map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/acct/access/apps":
			writeEnvelope(w, []any{})
		case r.Method == http.MethodPost && r.URL.Path == "/accounts/acct/access/apps":
			appBody = body
			writeEnvelope(w, map[string]any{"id": "app_1", "domain": body["domain"]})
		default:
			http.Error(w, `{"success":false}`, http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := New(Config{Token: "secret", AccountID: "acct", BaseURL: server.URL + "/"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.ApplyWebRoute(context.Background(), provision.RouteSpec{TunnelID: "tun_1", Private: true}); err != nil {
		t.Fatalf("private route config: %v", err)
	}
	route, err := client.EnsurePrivateRoute(context.Background(), provision.PrivateRouteSpec{Network: "192.168.1.20/32", TunnelID: "tun_1", Comment: "LAN app"})
	if err != nil {
		t.Fatalf("private route: %v", err)
	}
	if route.ID != "route_1" || routeBody["network"] != "192.168.1.20/32" {
		t.Fatalf("route = %+v body=%v", route, routeBody)
	}
	app, err := client.EnsureApplication(context.Background(), provision.AccessApplicationSpec{Name: "LAN app", Domain: "192.168.1.20:8080", Private: true})
	if err != nil {
		t.Fatalf("private access application: %v", err)
	}
	if app.ID != "app_1" {
		t.Fatalf("application = %+v", app)
	}
	if appBody["allow_authenticate_via_warp"] != true {
		t.Fatalf("warp auth body = %v", appBody)
	}
	if appBody["domain"] != "192.168.1.20" || appBody["domain_type"] != "private" {
		t.Fatalf("private application domain = %v", appBody)
	}
	destinations, ok := appBody["destinations"].([]any)
	if !ok || len(destinations) != 1 {
		t.Fatalf("private destinations = %v", appBody["destinations"])
	}
	destination, ok := destinations[0].(map[string]any)
	if !ok || destination["type"] != "private" || destination["cidr"] != "192.168.1.20/32" || destination["port_range"] != "8080" {
		t.Fatalf("private destination = %v", destinations[0])
	}
}

func TestClientDeletesOwnedResourcesAndTreatsMissingResourcesAsSuccess(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete && r.URL.Path == "/accounts/acct/cfd_tunnel/missing" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1003,"message":"not found"}]}`))
			return
		}
		writeEnvelope(w, map[string]any{})
	}))
	defer server.Close()

	client, err := New(Config{Token: "secret", AccountID: "acct", ZoneID: "zone", BaseURL: server.URL + "/"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.DeleteCNAME(context.Background(), "dns_1"); err != nil {
		t.Fatalf("delete dns: %v", err)
	}
	if err := client.DeletePolicy(context.Background(), "app_1", "policy_1"); err != nil {
		t.Fatalf("delete policy: %v", err)
	}
	if err := client.DeleteApplication(context.Background(), "app_1"); err != nil {
		t.Fatalf("delete application: %v", err)
	}
	if err := client.DeletePrivateRoute(context.Background(), "route_1"); err != nil {
		t.Fatalf("delete private route: %v", err)
	}
	if err := client.DeleteTunnel(context.Background(), "tun_1"); err != nil {
		t.Fatalf("delete tunnel: %v", err)
	}
	if err := client.DeleteTunnel(context.Background(), "missing"); err != nil {
		t.Fatalf("delete missing tunnel: %v", err)
	}

	want := []string{
		"DELETE /zones/zone/dns_records/dns_1",
		"DELETE /accounts/acct/access/apps/app_1/policies/policy_1",
		"DELETE /accounts/acct/access/apps/app_1",
		"DELETE /accounts/acct/teamnet/routes/route_1",
		"DELETE /accounts/acct/cfd_tunnel/tun_1",
		"DELETE /accounts/acct/cfd_tunnel/missing",
	}
	if !strings.EqualFold(strings.Join(paths, "\n"), strings.Join(want, "\n")) {
		t.Fatalf("delete paths = %v, want %v", paths, want)
	}
}

func TestPrivateApplicationTarget(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		host    string
		cidr    string
		port    string
		wantErr bool
	}{
		{name: "ipv4", domain: "192.168.1.20:8080", host: "192.168.1.20", cidr: "192.168.1.20/32", port: "8080"},
		{name: "ipv6", domain: "[fd00::20]:8443", host: "fd00::20", cidr: "fd00::20/128", port: "8443"},
		{name: "missing port", domain: "192.168.1.20", wantErr: true},
		{name: "invalid port", domain: "192.168.1.20:65536", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, cidr, port, err := privateApplicationTarget(tt.domain)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("privateApplicationTarget(%q) succeeded", tt.domain)
				}
				return
			}
			if err != nil {
				t.Fatalf("privateApplicationTarget(%q): %v", tt.domain, err)
			}
			if host != tt.host || cidr != tt.cidr || port != tt.port {
				t.Fatalf("privateApplicationTarget(%q) = %q, %q, %q", tt.domain, host, cidr, port)
			}
		})
	}
}
