# ADR-005: Device Runtime & State Engine

- **Date:** 2026-05-10
- **Status:** Proposed
- **Deciders:** Architecture team + AI Agent (autonomous L4)
- **Depends on:** ADR-001..004 (registry, matcher, parsers, capabilities)

## Context

Device-urile platformei publica telemetrie pe MQTT, dar **NU avem track in-memory al starii lor**:
- N-avem `last_seen` per device (cand a publicat ultima oara)
- N-avem detectie online/offline automata
- Frontend nu poate afișa `🟢 online` / `🔴 offline` fără query Influx pe range
- Rules engine nu poate genera alert "device X offline > 5 min"
- Restart Go ingest = pierdere completă a stării in-memory (și nimeni nu știe că un device n-a mai publicat de 1h)

Probleme:
- Frontend-ul polling periodic pe `/metrics/{device}/{field}` ca să ghicească if device live
- Rules nu pot lega de evenimente runtime (offline trigger)
- Deciziile (ex: `notify owner if SUN2000 offline > 30 min`) imposibile fără state

## Decision

Introducem **Device Runtime + State Engine** ca layer in-memory cross-cutting peste ingest pipeline:

```
MQTT msg → matcher → parser → ParsedTelemetry
                                      ↓
                                runtime.OnTelemetry(deviceID, fields)
                                      ↓                ↘
                                Influx write       Update DeviceRuntime{
                                                     last_seen: time.Now(),
                                                     online: true,
                                                     last_fields: fields,
                                                     capabilities: dd.ResolvedCapabilities,
                                                   }
                                                      ↓
                                                  Redis sync (write-through)
                                                  TTL 10min, key=runtime:{serial}
```

Background goroutine `offlineDetector` rulează la 30s și marchează `online=false` daca `last_seen + offline_after < now`. `offline_after` e configurabil per DD (default 3min pentru telemetry, 4h pentru zigbee battery-powered).

### `DeviceRuntime` schema

```go
type DeviceRuntime struct {
    DeviceID     string              `json:"device_id"`
    TenantID     int64               `json:"tenant_id"`
    Capabilities []string            `json:"capabilities"`     // resolved (cu inheritance)
    LastSeen     time.Time           `json:"last_seen"`
    Online       bool                `json:"online"`
    LastStream   string              `json:"last_stream"`      // sensor / telemetry / state
    LastSource   string              `json:"last_source"`      // nousat / sun2000 / shelly
    LastFields   map[string]any      `json:"last_fields"`      // ultimele field-uri parsate
    OfflineAfter time.Duration       `json:"offline_after_s"`  // threshold per DD
    UpdatedAt    time.Time           `json:"updated_at"`
}
```

### `RuntimeManager` API

```go
type RuntimeManager struct {
    mu      sync.RWMutex
    devices map[string]*DeviceRuntime  // keyed by deviceID
    redis   *redis.Client              // optional cross-instance sync
}

// OnTelemetry e apelat din cmd/main.go la fiecare parser hit reușit.
func (m *RuntimeManager) OnTelemetry(deviceID string, tenantID int64,
    caps []string, stream, source string, fields map[string]any,
    offlineAfter time.Duration) {...}

// Get state pentru un device.
func (m *RuntimeManager) Get(deviceID string) (*DeviceRuntime, bool) {...}

// All filter pe tenant.
func (m *RuntimeManager) ByTenant(tenantID int64) []*DeviceRuntime {...}

// Filter capabilities (uses runtime caps, not DD direct).
func (m *RuntimeManager) ByCapability(tenantID int64, cap string) []*DeviceRuntime {...}

// MarkOffline marchează device offline (apelat de offlineDetector goroutine).
func (m *RuntimeManager) MarkOffline(deviceID string) {...}

// SnapshotAll — pentru API list endpoint.
func (m *RuntimeManager) SnapshotAll() []*DeviceRuntime {...}
```

### Offline detection algorithm

```
every 30 seconds:
  now = time.Now()
  for each device in manager.devices:
    if device.online && device.last_seen + device.offline_after < now:
      device.online = false
      log structured event "device offline" (tenant_id, device_id, last_seen)
      // Faza 8: emit event pe rule engine bus
```

### Redis sync (cross-instance)

Pentru deploy-uri cu multiple instanțe Go ingest (load balancing MQTT shared subs):
- Pe `OnTelemetry`: `SET runtime:{serial} <json> EX 600` (TTL 10min, refresh la fiecare update)
- Pe `Get` cache miss: `GET runtime:{serial}` → reconstruct DeviceRuntime
- Pe restart: `SCAN runtime:*` populează in-memory map
- Pe `MarkOffline`: update Redis cu `online=false`

În caz Redis indisponibil → fallback la in-memory only (acceptabil pentru single-instance).

### API endpoints (noi)

```
GET /go/runtime              → list all runtimes pentru tenant (filter automat din JWT)
GET /go/runtime/{device}     → state pentru un device specific
GET /go/runtime?capability=X → filter pe capability
```

Toate cu autorizare JWT (tenant scope strict). Răspuns cache 1s la edge (Kong).

### Frontend integration

Frontend `DevicesPage` va polui `/go/runtime` la 5s și va afișa indicator vizual:
- 🟢 online (last_seen < 1min): pulsing green dot
- 🟡 stale (last_seen 1-5min): yellow
- 🔴 offline (last_seen > offline_after): red

`SolarPage` și `BoilerPage` deja afișează "live" indicator — îl conectăm la runtime in loc de presupunere bazat pe data freshness.

## Alternatives Considered

### A. Database-backed (MySQL Device.last_seen + cron mark offline)
**Pro:** Persistent fără Redis.
**Con:** Write per mesaj MQTT = ~10K writes/s la scale → MySQL bottleneck. Cron polling = max 1 min delay detectie offline.
**Decizie:** Reject pentru hot path. MySQL OK pentru Audit log offline events (Faza 8).

### B. InfluxDB-derived state (query "last point per device" la fiecare cerere)
**Pro:** Single source of truth.
**Con:** Query expensive cu N device-uri × M tenants. Cache layer obligatoriu → reinventăm runtime.
**Decizie:** Reject. Influx e pentru istorice, runtime e pentru "now".

### C. Pure Redis (no in-memory)
**Pro:** Cross-instance natural.
**Con:** Lookup latency 0.3-1ms × N device-uri pentru list view = slow. In-memory + Redis sync e cel mai bun trade-off.
**Decizie:** Hybrid (in-memory primary, Redis sync).

### D. Externalize la HA-like state actor (Erlang/Elixir style)
**Pro:** Concurrent-safe by design.
**Con:** Tech stack inconsistent cu Go. Over-engineering pentru scale curent.
**Decizie:** Reject — Go sync.Map suficient.

## Consequences

### Positive
- **Real-time live status** — frontend afișează correct online/offline fără ghicit
- **Foundation pentru Rules** — Faza 8 poate face `if device.online == false for 30min then notify`
- **Foundation pentru SLA tracking** — uptime per device calculabil
- **Debug îmbunătățit** — `GET /go/runtime/{device}` arată ultimul flux

### Negative
- **Memory:** ~1KB/device × 10K = 10MB per Go instance. Acceptabil.
- **Race conditions:** map writes concurrente → sync.RWMutex (low contention pe RW pattern read-heavy).
- **Restart blast** — la restart Go, runtime e gol până când fiecare device publică din nou (max 5 min pentru SUN2000 telemetry; max 4h pentru Z2M battery). Mitigare: Redis populate la startup.

### Neutral
- **Storage:** Redis ~100KB la 10K devices, TTL refresh la fiecare msg.

## Implementation Plan

### Faza 6 (acest ADR)

1. Go `internal/runtime/`:
   - `types.go` — `DeviceRuntime` struct, helpers
   - `manager.go` — `RuntimeManager` cu sync.RWMutex + Redis client opțional
   - `detector.go` — background goroutine offline detection
   - `runtime_test.go` — concurrent updates, offline detection, Redis sync mock
2. `cmd/main.go` integration:
   - Init RuntimeManager la startup (după Redis)
   - Apelare `runtime.OnTelemetry(...)` în `handleMessage` după `parsers.Parse()` (data streams)
   - Start offlineDetector goroutine
3. `internal/api/handlers.go` extend:
   - `GET /metrics/runtime` (lista) — filtrare tenant din JWT
   - `GET /metrics/runtime/{device}` (specific)
   - `GET /metrics/runtime?capability=X`
4. Frontend:
   - `lib/api.ts`: `useDeviceRuntime(serial)` hook query la 5s
   - `DevicesPage` adăugare coloană "Status" cu pulse indicator
   - Tests E2E: când device n-a publicat 5min → 🔴 offline

### Out of scope

- Faza 8: Rule engine consumă runtime events
- Audit log integration (offline event → AuditLog) — separat
- OpenTelemetry tracing runtime updates — Faza 7

## Validation

- Test coverage > 85% pe `internal/runtime/`
- Bench: `OnTelemetry` < 5µs (hot path)
- Integration: device offline detected < 60s post-disconnect (configurabil)
- Memory: 10K devices = < 50MB heap

## References

- ADR-001..004
- Roadmap §Faza 6
- Inspirație: Home Assistant `state_machine`, Eclipse Vorto runtime
