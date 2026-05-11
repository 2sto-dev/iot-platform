package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"go-iot-platform/internal/runtime"
)

// TestStripPrefixGoMount — regression: serverul mount-uiește mux-ul sub /go cu
// http.StripPrefix("/go", mux). Dacă cineva strip-uiește /go ÎN PLUS la Kong
// (strip_path: true) toate request-urile cad pe catch-all 404 — ceea ce s-a
// întâmplat în branch-ul agent/fix-kong-go-route înainte de revert.
//
// Acest test apără invariantul: numai cererile cu prefix /go ajung la handler.
func TestStripPrefixGoMount(t *testing.T) {
	const secret = "routing-test-secret"
	t.Setenv("JWT_SECRET", secret)

	mux := http.NewServeMux()
	RegisterRoutes(mux, runtime.New(nil))

	// Replică mount-ul din cmd/main.go: handler-ul total = StripPrefix("/go", mux).
	handler := http.StripPrefix("/go", mux)

	token := signedToken(t, secret, jwt.MapClaims{
		"username":  "alice",
		"tenant_id": 1,
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	cases := []struct {
		name string
		path string
		want int // doar prefixul; 401 = handler ajuns, 404 = StripPrefix/mux n-a găsit ruta
	}{
		{"runtime fără prefix /go", "/runtime", http.StatusNotFound},
		{"runtime cu prefix /go", "/go/runtime", http.StatusOK},
		{"runtime/{id} fără prefix", "/runtime/X1", http.StatusNotFound},
		{"runtime/{id} cu prefix", "/go/runtime/X1", http.StatusNotFound}, // 404 = device not found (handler ajuns)
		{"metrics fără prefix", "/metrics/X1/power", http.StatusNotFound},
		{"metrics cu prefix dar invalid path", "/go/metrics/no-field", http.StatusBadRequest},
		{"path inexistent cu prefix", "/go/whatever", http.StatusNotFound}, // mux catch-all
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("%s: got %d, want %d (body=%q)", tc.path, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// TestRuntimeListRequiresAuth — fără JWT → 401, indiferent de prefix.
func TestRuntimeListRequiresAuth(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, runtime.New(nil))
	handler := http.StripPrefix("/go", mux)

	req := httptest.NewRequest(http.MethodGet, "/go/runtime", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}
}

// TestRuntimeListReturnsTenantScoped — verifică izolarea pe tenant.
func TestRuntimeListReturnsTenantScoped(t *testing.T) {
	const secret = "tenant-test-secret"
	t.Setenv("JWT_SECRET", secret)

	rtMgr := runtime.New(nil)
	rtMgr.OnTelemetry("dev-alice", 1, []string{"power_meter"}, "emeter", "mqtt", nil, 0)
	rtMgr.OnTelemetry("dev-bob", 2, []string{"power_meter"}, "emeter", "mqtt", nil, 0)

	mux := http.NewServeMux()
	RegisterRoutes(mux, rtMgr)
	handler := http.StripPrefix("/go", mux)

	tokenAlice := signedToken(t, secret, jwt.MapClaims{
		"username":  "alice",
		"tenant_id": 1,
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/go/runtime", nil)
	req.Header.Set("Authorization", "Bearer "+tokenAlice)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, "dev-alice") {
		t.Errorf("alice's device not in response: %s", body)
	}
	if contains(body, "dev-bob") {
		t.Errorf("LEAK: bob's device returned to alice: %s", body)
	}
}

// TestRuntimeGetCrossTenant — 404 (no leak) când device-ul aparține altui tenant.
func TestRuntimeGetCrossTenant(t *testing.T) {
	const secret = "xt-test-secret"
	t.Setenv("JWT_SECRET", secret)

	rtMgr := runtime.New(nil)
	rtMgr.OnTelemetry("dev-bob", 2, []string{"power_meter"}, "emeter", "mqtt", nil, 0)

	mux := http.NewServeMux()
	RegisterRoutes(mux, rtMgr)
	handler := http.StripPrefix("/go", mux)

	tokenAlice := signedToken(t, secret, jwt.MapClaims{
		"username":  "alice",
		"tenant_id": 1,
		"exp":       time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/go/runtime/dev-bob", nil)
	req.Header.Set("Authorization", "Bearer "+tokenAlice)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 cross-tenant (no leak), got %d", rec.Code)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
