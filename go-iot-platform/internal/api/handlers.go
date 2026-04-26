package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"go-iot-platform/internal/config"
	"go-iot-platform/internal/django"
	"go-iot-platform/internal/influx"
)

// Înregistrăm rutele API-ului Go.
func RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/metrics/", http.HandlerFunc(metricsHandler))
}

// Claims-urile relevante extrase din JWT după validarea făcută de Kong.
type tokenContext struct {
	Username   string
	TenantID   int64
	TenantSlug string
	Role       string
}

func getTokenContext(r *http.Request) (tokenContext, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return tokenContext{}, fmt.Errorf("missing bearer token")
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.Get("JWT_SECRET")), nil
	})
	if err != nil || !token.Valid {
		return tokenContext{}, fmt.Errorf("invalid token: %v", err)
	}

	username, _ := claims["username"].(string)
	if username == "" {
		return tokenContext{}, fmt.Errorf("username missing in token")
	}

	ctx := tokenContext{Username: username}
	if v, ok := claims["tenant_id"].(float64); ok {
		ctx.TenantID = int64(v)
	}
	if s, ok := claims["tenant_slug"].(string); ok {
		ctx.TenantSlug = s
	}
	if s, ok := claims["role"].(string); ok {
		ctx.Role = s
	}
	if ctx.TenantID == 0 {
		return tokenContext{}, fmt.Errorf("tenant_id missing in token")
	}
	return ctx, nil
}

// GET /go/metrics/{device}/{field}?range=15m
// JWT validat de Kong; Go re-decodează ca să extragă tenant_id + verifică în Django
// că device-ul e al userului IN tenant-ul curent, apoi citește din Influx (filtrat pe tenant_id).
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("👉 Request primit: %s %s", r.Method, r.URL.Path)

	tc, err := getTokenContext(r)
	if err != nil {
		log.Printf("❌ JWT error: %v", err)
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}
	log.Printf("✅ Token: user=%s tenant=%d (%s) role=%s", tc.Username, tc.TenantID, tc.TenantSlug, tc.Role)

	path := strings.TrimPrefix(r.URL.Path, "/metrics/")
	segments := strings.Split(path, "/")
	if len(segments) != 2 {
		http.Error(w, "Invalid metric path. Use /metrics/{device}/{field}", http.StatusBadRequest)
		return
	}
	device := segments[0]
	field := segments[1]

	devices, err := django.GetDevicesForUserInTenant(tc.Username, tc.TenantID)
	if err != nil {
		log.Printf("❌ Django error: %v", err)
		http.Error(w, "Django error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	allowed := false
	for _, d := range devices {
		if d.Serial == device {
			allowed = true
			break
		}
	}
	if !allowed {
		log.Printf("⛔ user=%s tenant=%d nu are acces la device=%s", tc.Username, tc.TenantID, device)
		http.Error(w, "Device not allowed for user/tenant", http.StatusForbidden)
		return
	}

	rangeStr := r.URL.Query().Get("range")
	val, err := influx.GetFieldForDevice(device, field, rangeStr, tc.TenantID)
	if err != nil {
		log.Printf("❌ Influx error pentru %s/%s: %v", device, field, err)
		http.Error(w, "Influx error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"device":    device,
		"field":     field,
		"value":     val,
		"tenant_id": tc.TenantID,
	})
}
