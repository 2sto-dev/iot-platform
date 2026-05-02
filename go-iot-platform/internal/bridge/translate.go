// Package bridge translatează topicuri MQTT vendor-shaped (legacy) în schema tenant-aware
// `tenants/{tenant_id}/devices/{device_id}/up/{stream}[/...]`.
//
// Folosit de cmd/mqtt-bridge: subscribe pe pattern legacy → Translate() → publish înapoi
// pe brokerul EMQX (același broker; bridge-ul e un client MQTT, nu un broker separat).
package bridge

import (
	"fmt"
	"strings"
)

// Result e rezultatul translatării unui topic legacy.
type Result struct {
	NewTopic string
	DeviceID string
	Stream   string
	Skip     bool   // true dacă topicul nu poate fi translatat (unknown vendor, malformed)
	Reason   string // human-readable reason când Skip=true
}

// Translate transformă un topic vendor în schema tenant-aware. Necesită tenant_id-ul
// device-ului (looked up via cache de caller).
//
// Reguli:
//   - shellies/{serial}/...               → tenants/{tid}/devices/{serial}/up/{...}      (preserve subpath; stream="emeter"|"relay"|...)
//   - tele/{serial}/STATE | SENSOR | ... → tenants/{tid}/devices/{serial}/up/STATE|...   (preserve suffix exact pentru handleMessage)
//   - zigbee2mqtt/{serial}                → tenants/{tid}/devices/{serial}/up/zigbee     (no further path)
//   - tenants/...                          → Skip (deja tenant-scoped, evită loop)
//   - orice altceva                        → Skip (vendor necunoscut)
func Translate(legacyTopic string, tenantID int64) Result {
	if tenantID <= 0 {
		return Result{Skip: true, Reason: "invalid tenant_id (≤0)"}
	}
	if strings.HasPrefix(legacyTopic, "tenants/") {
		return Result{Skip: true, Reason: "already tenant-scoped"}
	}

	parts := strings.Split(legacyTopic, "/")
	if len(parts) < 2 {
		return Result{Skip: true, Reason: "topic too short (need at least vendor/serial)"}
	}

	vendor := parts[0]
	serial := parts[1]
	if serial == "" {
		return Result{Skip: true, Reason: "empty serial"}
	}

	switch vendor {
	case "shellies":
		// shellies/{serial}/X/Y/Z → tenants/{tid}/devices/{serial}/up/X/Y/Z
		if len(parts) < 3 {
			return Result{Skip: true, Reason: "shelly topic missing subpath"}
		}
		rest := strings.Join(parts[2:], "/")
		newTopic := fmt.Sprintf("tenants/%d/devices/%s/up/%s", tenantID, serial, rest)
		return Result{NewTopic: newTopic, DeviceID: serial, Stream: parts[2]}

	case "tele":
		// tele/{serial}/STATE → tenants/{tid}/devices/{serial}/up/STATE  (suffix preserved exactly)
		if len(parts) < 3 {
			return Result{Skip: true, Reason: "tele topic missing suffix"}
		}
		rest := strings.Join(parts[2:], "/")
		newTopic := fmt.Sprintf("tenants/%d/devices/%s/up/%s", tenantID, serial, rest)
		return Result{NewTopic: newTopic, DeviceID: serial, Stream: parts[2]}

	case "zigbee2mqtt":
		// zigbee2mqtt/{serial}        → tenants/{tid}/devices/{serial}/up/zigbee
		// zigbee2mqtt/{serial}/X       → tenants/{tid}/devices/{serial}/up/zigbee/X (păstrăm subpath dacă există)
		newTopic := fmt.Sprintf("tenants/%d/devices/%s/up/zigbee", tenantID, serial)
		if len(parts) > 2 {
			newTopic += "/" + strings.Join(parts[2:], "/")
		}
		return Result{NewTopic: newTopic, DeviceID: serial, Stream: "zigbee"}

	default:
		return Result{Skip: true, Reason: fmt.Sprintf("unknown vendor: %s", vendor)}
	}
}

// LegacyPatterns returnează lista de topicuri MQTT pe care bridge-ul trebuie să le subscrie
// (NOT shared subscription — bridge-ul rulează ca instanță unică pentru a evita dublare).
func LegacyPatterns() []string {
	return []string{
		"shellies/+/#",
		"tele/+/#",
		"zigbee2mqtt/+",
		"zigbee2mqtt/+/#",
	}
}
