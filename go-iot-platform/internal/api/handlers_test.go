package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func signedToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

func TestGetTokenContext(t *testing.T) {
	const secret = "unit-test-secret"
	t.Setenv("JWT_SECRET", secret)
	os.Setenv("JWT_SECRET", secret)

	now := time.Now().Add(time.Hour).Unix()

	cases := []struct {
		name       string
		authHeader string
		wantErr    bool
		wantUser   string
		wantTenant int64
	}{
		{
			name: "valid with tenant",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{
				"username":    "alice",
				"tenant_id":   42,
				"tenant_slug": "acme",
				"role":        "OWNER",
				"exp":         now,
			}),
			wantUser:   "alice",
			wantTenant: 42,
		},
		{
			name: "missing tenant_id",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{
				"username": "alice",
				"exp":      now,
			}),
			wantErr: true,
		},
		{
			name:       "missing header",
			authHeader: "",
			wantErr:    true,
		},
		{
			name:       "wrong scheme",
			authHeader: "Basic abc",
			wantErr:    true,
		},
		{
			name: "bad signature",
			authHeader: "Bearer " + signedToken(t, "other-secret", jwt.MapClaims{
				"username": "alice", "tenant_id": 1, "exp": now,
			}),
			wantErr: true,
		},
		{
			name: "expired",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{
				"username": "alice", "tenant_id": 1,
				"exp": time.Now().Add(-time.Hour).Unix(),
			}),
			wantErr: true,
		},
		{
			name: "username missing",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{
				"tenant_id": 1, "exp": now,
			}),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics/foo/bar", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			ctx, err := getTokenContext(req)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", ctx)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ctx.Username != tc.wantUser {
				t.Errorf("user = %q, want %q", ctx.Username, tc.wantUser)
			}
			if ctx.TenantID != tc.wantTenant {
				t.Errorf("tenant = %d, want %d", ctx.TenantID, tc.wantTenant)
			}
		})
	}
}
