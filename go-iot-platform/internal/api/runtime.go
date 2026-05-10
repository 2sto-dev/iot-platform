package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// runtimeListHandler — GET /go/runtime?capability=X
//
// Returnează toate device-urile tenant-ului curent (din JWT). Dacă `?capability=X`,
// filtrăm la cele ce includ capability X (cu inheritance).
//
// Răspuns: [{device_id, online, last_seen, capabilities, ...}, ...]
func runtimeListHandler(w http.ResponseWriter, r *http.Request) {
	tc, err := getTokenContext(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if rtMgr == nil {
		http.Error(w, "Runtime manager not configured", http.StatusServiceUnavailable)
		return
	}

	capability := r.URL.Query().Get("capability")
	var list []*struct{}
	_ = list // placeholder

	devices := rtMgr.ByTenant(tc.TenantID)
	if capability != "" {
		filtered := devices[:0]
		for _, d := range devices {
			if d.HasCapability(capability) {
				filtered = append(filtered, d)
			}
		}
		devices = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=1")
	if err := json.NewEncoder(w).Encode(devices); err != nil {
		log.Printf("runtime list encode: %v", err)
	}
}

// runtimeGetHandler — GET /go/runtime/{device}
//
// Returnează state-ul pentru un device specific dacă aparține tenant-ului din JWT.
// 404 dacă nu există în runtime; 403 dacă există dar e cross-tenant.
func runtimeGetHandler(w http.ResponseWriter, r *http.Request) {
	tc, err := getTokenContext(r)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if rtMgr == nil {
		http.Error(w, "Runtime manager not configured", http.StatusServiceUnavailable)
		return
	}

	deviceID := strings.TrimPrefix(r.URL.Path, "/runtime/")
	deviceID = strings.TrimSuffix(deviceID, "/")
	if deviceID == "" {
		http.Error(w, "Missing device id", http.StatusBadRequest)
		return
	}

	d, ok := rtMgr.Get(deviceID)
	if !ok {
		http.Error(w, "Device not in runtime (no telemetry yet?)", http.StatusNotFound)
		return
	}
	if d.TenantID != tc.TenantID {
		// Tenant isolation strictă — același mesaj ca pentru not found,
		// nu leak-ăm existența device-ului altui tenant.
		http.Error(w, "Device not in runtime (no telemetry yet?)", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=1")
	_ = json.NewEncoder(w).Encode(d)
}
