// Package topics parses and validates MQTT topic strings.
//
// New schema (Faza 2.1+): tenants/{tenant_id}/devices/{device_id}/{up|down}/{stream}[/...]
// Legacy schema (current device-uri vendor-shaped): orice altceva (shellies/.../, tele/.../, etc.)
//
// Parserul e DEFENSIV: dacă topic-ul începe cu "tenants/", validează strict; altfel
// întoarce IsLegacy=true și caller-ul rămâne pe flow-ul existent (lookup device→tenant
// via Django). Astfel nu spargem telemetria curentă înainte de migrarea schemei (Faza 2.1).
package topics

import (
	"fmt"
	"strconv"
	"strings"
)

const NewSchemaPrefix = "tenants/"

type Parsed struct {
	IsLegacy  bool
	TenantID  int64
	DeviceID  string
	Direction string // "up" sau "down"
	Stream    string // primul segment după direction; pentru topicuri mai adânci, restul e ignorat
	Raw       string
}

func Parse(topic string) (Parsed, error) {
	p := Parsed{Raw: topic}

	if !strings.HasPrefix(topic, NewSchemaPrefix) {
		p.IsLegacy = true
		return p, nil
	}

	parts := strings.Split(topic, "/")
	// tenants / {tid} / devices / {did} / {up|down} / {stream} [...]
	if len(parts) < 6 {
		return p, fmt.Errorf("malformed tenant-scoped topic (need ≥6 segments): %q", topic)
	}
	if parts[0] != "tenants" || parts[2] != "devices" {
		return p, fmt.Errorf("malformed tenant-scoped topic (bad layout): %q", topic)
	}
	if parts[1] == "" || parts[3] == "" {
		return p, fmt.Errorf("empty tenant_id or device_id in topic: %q", topic)
	}
	if parts[4] != "up" && parts[4] != "down" {
		return p, fmt.Errorf("invalid direction (need up|down): %q", topic)
	}

	tid, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return p, fmt.Errorf("tenant_id not numeric in topic %q: %v", topic, err)
	}
	if tid <= 0 {
		return p, fmt.Errorf("tenant_id must be positive in topic %q", topic)
	}

	p.TenantID = tid
	p.DeviceID = parts[3]
	p.Direction = parts[4]
	p.Stream = parts[5]
	return p, nil
}

// LegacyDeviceID extracts the device serial from a legacy topic. Returns "" if the
// topic doesn't have at least 2 segments (caller should drop in that case).
func LegacyDeviceID(topic string) string {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
