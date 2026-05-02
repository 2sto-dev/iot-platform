// Package bridge translates legacy vendor MQTT topics to the tenant-scoped schema.
//
// Legacy topic shapes:
//   shellies/{serial}/{stream...}  — Shelly devices (EM, Plus, etc.)
//   tele/{serial}/{stream}         — Tasmota / NousAT
//   zigbee2mqtt/{serial}           — Zigbee2MQTT gateway
//
// Output schema:
//   tenants/{tenant_id}/devices/{serial}/up/{stream}
package bridge

import (
	"fmt"
	"strings"
)

// ParseLegacy extracts serial and stream name from a vendor-shaped topic.
// Returns ok=false for unrecognized prefixes or malformed topics.
func ParseLegacy(topic string) (serial, stream string, ok bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 || parts[1] == "" {
		return "", "", false
	}
	switch parts[0] {
	case "shellies":
		if len(parts) >= 3 && parts[2] != "" {
			return parts[1], strings.ToLower(parts[2]), true
		}
		return parts[1], "status", true
	case "tele":
		if len(parts) >= 3 && parts[2] != "" {
			return parts[1], strings.ToLower(parts[2]), true
		}
		return parts[1], "tele", true
	case "zigbee2mqtt":
		return parts[1], "zigbee", true
	}
	return "", "", false
}

// NewTopic builds the tenant-scoped publish topic for a translated legacy message.
func NewTopic(tenantID int64, serial, stream string) string {
	return fmt.Sprintf("tenants/%d/devices/%s/up/%s", tenantID, serial, stream)
}
