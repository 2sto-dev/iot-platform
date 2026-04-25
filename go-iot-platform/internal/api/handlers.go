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

// Înregistrăm toate rutele API-ului Go
func RegisterRoutes(mux *http.ServeMux) {
	// Endpoint principal: valori metrice pentru un device
	mux.Handle("/metrics/", http.HandlerFunc(metricsHandler))
}

// extrage username din JWT (Authorization: Bearer <token>)
func getUsernameFromToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", fmt.Errorf("missing bearer token")
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.Get("JWT_SECRET")), nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid token: %v", err)
	}

	username, ok := claims["username"].(string)
	if !ok || username == "" {
		return "", fmt.Errorf("username missing in token")
	}
	return username, nil
}

// GET /go/metrics/{device}/{field}
// - JWT validat de Kong, Go decodează și extrage username
// - Verifică în Django dacă userul are acces la device
// - Citește ultima valoare din InfluxDB pentru acel câmp
func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// log de debug pentru request
	log.Printf("👉 Request primit: %s %s", r.Method, r.URL.Path)
	log.Printf("👉 Headers: %+v", r.Header)

	username, err := getUsernameFromToken(r)
	if err != nil {
		log.Printf("❌ JWT error: %v", err)
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}
	log.Printf("✅ User extras din token: %s", username)

	// extragem device și field din path
	path := strings.TrimPrefix(r.URL.Path, "/metrics/")
	segments := strings.Split(path, "/")
	if len(segments) != 2 {
		log.Printf("❌ Path invalid: %s", path)
		http.Error(w, "Invalid metric path. Use /metrics/{device}/{field}", http.StatusBadRequest)
		return
	}
	device := segments[0]
	field := segments[1]
	log.Printf("🔎 Device: %s, Field: %s", device, field)

	// verificăm în Django dacă userul are acces
	devices, err := django.GetDevicesForUser(username)
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
		log.Printf("⛔ User %s NU are acces la device %s", username, device)
		http.Error(w, "Device not allowed for user", http.StatusForbidden)
		return
	}
	log.Printf("✅ User %s are acces la device %s", username, device)

	// citim valoarea din Influx (range opțional din query string, default -5m)
	rangeStr := r.URL.Query().Get("range")
	val, err := influx.GetFieldForDevice(device, field, rangeStr)
	if err != nil {
		log.Printf("❌ Influx error pentru %s/%s: %v", device, field, err)
		http.Error(w, "Influx error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("📊 Valoare din Influx pentru %s/%s: %v", device, field, val)

	// răspuns JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device": device,
		"field":  field,
		"value":  val,
	})
}
