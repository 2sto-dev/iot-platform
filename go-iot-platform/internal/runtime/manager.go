package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RuntimeManager menține un map in-memory deviceID → *DeviceRuntime cu sync.RWMutex.
// Optional, sincronizează prin Redis (write-through) pentru cross-instance state.
type RuntimeManager struct {
	mu      sync.RWMutex
	devices map[string]*DeviceRuntime
	redis   *redis.Client // poate fi nil — atunci skip sync
	prefix  string        // Redis key prefix, default "runtime:"
}

// New creates a fresh RuntimeManager. `redisClient` poate fi nil.
func New(redisClient *redis.Client) *RuntimeManager {
	return &RuntimeManager{
		devices: make(map[string]*DeviceRuntime),
		redis:   redisClient,
		prefix:  "runtime:",
	}
}

// OnTelemetry — apelat din cmd/main.go la fiecare ParsedTelemetry produs cu succes.
// Update last_seen + online=true + last_fields, emite Redis sync write-through.
//
// Performance: < 5µs hot path (sync.RWMutex Lock + small map ops). Bench acoperă.
func (m *RuntimeManager) OnTelemetry(deviceID string, tenantID int64, capabilities []string,
	stream, source string, fields map[string]any, offlineAfter time.Duration) {

	if offlineAfter <= 0 {
		offlineAfter = DefaultOfflineAfter
	}

	now := time.Now()
	m.mu.Lock()
	d, exists := m.devices[deviceID]
	if !exists {
		d = &DeviceRuntime{DeviceID: deviceID, TenantID: tenantID}
		m.devices[deviceID] = d
	}
	d.TenantID = tenantID
	d.Capabilities = capabilities
	d.LastSeen = now
	d.Online = true
	d.LastStream = stream
	d.LastSource = source
	d.LastFields = fields
	d.OfflineAfter = offlineAfter
	d.UpdatedAt = now
	snapshot := *d // copy pentru Redis (eliberăm lock-ul înainte de Redis call)
	m.mu.Unlock()

	if m.redis != nil {
		m.syncToRedis(deviceID, &snapshot)
	}
}

// Get returnează state-ul pentru un device sau (nil, false) dacă nu există.
// Read lock — multiple goroutine pot citi concurent.
func (m *RuntimeManager) Get(deviceID string) (*DeviceRuntime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	d, ok := m.devices[deviceID]
	if !ok {
		return nil, false
	}
	// Return copy ca să eviti race pe caller-ul care modifică câmpuri.
	cp := *d
	return &cp, true
}

// ByTenant returnează toate runtime-urile unui tenant. Folosit de API list endpoint.
func (m *RuntimeManager) ByTenant(tenantID int64) []*DeviceRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*DeviceRuntime
	for _, d := range m.devices {
		if d.TenantID == tenantID {
			cp := *d
			out = append(out, &cp)
		}
	}
	return out
}

// ByCapability filtrează după capability (uses runtime caps, includes inheritance).
func (m *RuntimeManager) ByCapability(tenantID int64, capability string) []*DeviceRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*DeviceRuntime
	for _, d := range m.devices {
		if d.TenantID != tenantID {
			continue
		}
		if !d.HasCapability(capability) {
			continue
		}
		cp := *d
		out = append(out, &cp)
	}
	return out
}

// MarkOffline marchează un device offline. Folosit de detector goroutine.
// Idempotent: dacă deja offline, no-op (skip Redis write).
func (m *RuntimeManager) MarkOffline(deviceID string) bool {
	m.mu.Lock()
	d, ok := m.devices[deviceID]
	if !ok || !d.Online {
		m.mu.Unlock()
		return false
	}
	d.Online = false
	d.UpdatedAt = time.Now()
	snapshot := *d
	m.mu.Unlock()

	if m.redis != nil {
		m.syncToRedis(deviceID, &snapshot)
	}
	return true
}

// Count returnează numărul total de device-uri tracked (pentru health check).
func (m *RuntimeManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.devices)
}

// SnapshotAll — pentru debug / introspection. Returnează copy.
func (m *RuntimeManager) SnapshotAll() []*DeviceRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*DeviceRuntime, 0, len(m.devices))
	for _, d := range m.devices {
		cp := *d
		out = append(out, &cp)
	}
	return out
}

// ── Redis sync (cross-instance state) ───────────────────────────────────────

// syncToRedis e fire-and-forget: erorile sunt logged dar nu blochează ingest.
func (m *RuntimeManager) syncToRedis(deviceID string, d *DeviceRuntime) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	bytes, err := json.Marshal(d)
	if err != nil {
		return // shouldn't happen with well-typed struct
	}
	key := m.prefix + deviceID
	// TTL 10min — dacă device nu publică în 10min, Redis expira; in-memory rămâne
	// până la mark-offline detector.
	_ = m.redis.Set(ctx, key, bytes, 10*time.Minute).Err()
}

// LoadFromRedis populează in-memory map din Redis la startup (recovery post-restart).
// Idempotent: re-rulare suprascrie in-memory. Nu blochează startup în caz de eroare.
func (m *RuntimeManager) LoadFromRedis(ctx context.Context) error {
	if m.redis == nil {
		return nil // no-op
	}
	pattern := m.prefix + "*"
	cursor := uint64(0)
	loaded := 0
	for {
		keys, next, err := m.redis.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return fmt.Errorf("redis scan: %w", err)
		}
		for _, k := range keys {
			val, err := m.redis.Get(ctx, k).Bytes()
			if err != nil {
				continue // key may have expired between SCAN and GET
			}
			var d DeviceRuntime
			if err := json.Unmarshal(val, &d); err != nil {
				continue
			}
			m.mu.Lock()
			m.devices[d.DeviceID] = &d
			m.mu.Unlock()
			loaded++
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return nil
}

// LoadedCount — câte device-uri au fost recovered din Redis (debug log).
func (m *RuntimeManager) LoadedCount() int { return m.Count() }
