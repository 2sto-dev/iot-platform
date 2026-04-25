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

func TestGetUsernameFromToken(t *testing.T) {
	const secret = "unit-test-secret"
	t.Setenv("JWT_SECRET", secret)
	os.Setenv("JWT_SECRET", secret) // ensure config.Get sees it

	cases := []struct {
		name        string
		authHeader  string
		wantErr     bool
		wantUser    string
	}{
		{
			name:       "valid",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{"username": "alice", "exp": time.Now().Add(time.Hour).Unix()}),
			wantUser:   "alice",
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
			name:       "bad signature",
			authHeader: "Bearer " + signedToken(t, "other-secret", jwt.MapClaims{"username": "alice", "exp": time.Now().Add(time.Hour).Unix()}),
			wantErr:    true,
		},
		{
			name:       "expired",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{"username": "alice", "exp": time.Now().Add(-time.Hour).Unix()}),
			wantErr:    true,
		},
		{
			name:       "username missing",
			authHeader: "Bearer " + signedToken(t, secret, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}),
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics/foo/bar", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			user, err := getUsernameFromToken(req)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got user=%q", user)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user != tc.wantUser {
				t.Errorf("user = %q, want %q", user, tc.wantUser)
			}
		})
	}
}
