package runtime

import (
	"context"
	"log"
	"time"
)

// StartOfflineDetector lansează un goroutine background care periodic verifică
// dacă device-urile online au depășit `OfflineAfter` și le marchează offline.
//
// Frecvența de check: 30s (configurabil prin tickInterval).
//
// Goroutine-ul se oprește când ctx e cancelled. Returnează imediat (fire-and-forget).
func (m *RuntimeManager) StartOfflineDetector(ctx context.Context, tickInterval time.Duration) {
	if tickInterval <= 0 {
		tickInterval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.detectOfflineOnce()
			}
		}
	}()
}

// detectOfflineOnce iterează prin toate device-urile online și marchează offline
// pe cele cu last_seen prea vechi.
//
// Logging structured per device offline event. Faza 8 va emite acelasi event
// pe rule engine bus pentru notificări automate.
func (m *RuntimeManager) detectOfflineOnce() {
	now := time.Now()
	// Snapshot pentru iterație fără lock-ul ținut tot procesul
	m.mu.RLock()
	candidates := make([]*DeviceRuntime, 0)
	for _, d := range m.devices {
		if d.Online && d.LastSeen.Add(d.OfflineAfter).Before(now) {
			candidates = append(candidates, d)
		}
	}
	m.mu.RUnlock()

	for _, d := range candidates {
		// MarkOffline ia Lock pe scurt; race-ul cu OnTelemetry nou e rezolvat
		// de check-ul `!d.Online` în MarkOffline (idempotent).
		if m.MarkOffline(d.DeviceID) {
			log.Printf("[runtime] device offline: device_id=%s tenant_id=%d last_seen=%s offline_after=%s",
				d.DeviceID, d.TenantID,
				d.LastSeen.Format(time.RFC3339), d.OfflineAfter)
		}
	}
}
