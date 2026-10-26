package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sparklyi/bouncer"
	"github.com/sparklyi/bouncer/httpx"
	"github.com/sparklyi/bouncer/pow"
)

func TestLoginRequiresHumanCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gate, err := bouncer.New(bouncer.WithSecret(bytes.Repeat([]byte{6}, 32)), bouncer.WithDegree(bouncer.MinDegree))
	if err != nil {
		t.Fatalf("build gate: %v", err)
	}
	t.Cleanup(func() { _ = gate.Close() })

	router, err := NewRouter(Dependencies{
		Services: &fakeServiceActions{}, Operations: fakeOperationReader{}, Auth: testAuth(t),
		HumanCheck: httpx.NewGuard(gate),
	})
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	// A login without a pass is refused before the password is even checked.
	response := performJSONRequest(router, http.MethodPost, "/api/v1/auth/login", `{"password":"password123"}`, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("login without pass = %d, want 403: %s", response.Code, response.Body)
	}
	var payload struct{ Code string }
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if payload.Code != "challenge_required" {
		t.Fatalf("error code = %q, want challenge_required", payload.Code)
	}

	// Solve a challenge and exchange it for a pass cookie.
	response = performJSONRequest(router, http.MethodPost, "/_bouncer/challenge", `{}`, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("challenge = %d: %s", response.Code, response.Body)
	}
	var challenge struct {
		Token  string     `json:"token"`
		Params pow.Params `json:"params"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	answer, err := pow.Solve(context.Background(), challenge.Params)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	verifyBody, err := json.Marshal(map[string]string{"token": challenge.Token, "answer": answer})
	if err != nil {
		t.Fatal(err)
	}
	response = performJSONRequest(router, http.MethodPost, "/_bouncer/verify", string(verifyBody), nil)
	if response.Code != http.StatusOK {
		t.Fatalf("verify = %d: %s", response.Code, response.Body)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one pass cookie, got %d", len(cookies))
	}

	response = performJSONRequest(router, http.MethodPost, "/api/v1/auth/login", `{"password":"password123"}`, cookies[0])
	if response.Code != http.StatusNoContent {
		t.Fatalf("login with pass = %d, want 204: %s", response.Code, response.Body)
	}
}
