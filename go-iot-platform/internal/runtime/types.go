// Package runtime mentine starea in-memory a fiecarui device:
// last_seen, online status, capabilities, last fields, last stream.
//
// Foloseste un sync.RWMutex pentru concurrent map access (read-heavy pattern
// din API: list runtimes la 5s din frontend, write rar din ingest).
//
// Cross-instance sync optional via Redis (write-through pe OnTelemetry).
//
// Vezi: docs/adr/ADR-005-device-runtime.md
package runtime

import (
	"time"
)

// DeviceRuntime — starea curentă in-memory a unui device.
//
// JSON tags: expus prin API /go/runtime{device}. Frontend folosește online +
// last_seen pentru indicator visual.
type DeviceRuntime struct {
	DeviceID     string         `json:"device_id"`
	TenantID     int64          `json:"tenant_id"`
	Capabilities []string       `json:"capabilities"`             // resolved (cu inheritance)
	LastSeen     time.Time      `json:"last_seen"`
	Online       bool           `json:"online"`
	LastStream   string         `json:"last_stream,omitempty"`    // sensor / telemetry / state / etc
	LastSource   string         `json:"last_source,omitempty"`    // nousat / sun2000 / shelly / etc
	LastFields   map[string]any `json:"last_fields,omitempty"`    // ultimele field-uri parsate
	OfflineAfter time.Duration  `json:"offline_after_s"`          // threshold per DD (default 3min)
	UpdatedAt    time.Time      `json:"updated_at"`
}

// HasCapability check rapid; runtime preia capabilities din DD-ul matched.
func (d *DeviceRuntime) HasCapability(cap string) bool {
	for _, c := range d.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// AgeSinceLastSeen returnează durata scursă de la ultimul mesaj.
func (d *DeviceRuntime) AgeSinceLastSeen() time.Duration {
	if d.LastSeen.IsZero() {
		return -1
	}
	return time.Since(d.LastSeen)
}

// DefaultOfflineAfter — folosit dacă DD nu specifică în `telemetry_streams.offline_after`.
const DefaultOfflineAfter = 3 * time.Minute
