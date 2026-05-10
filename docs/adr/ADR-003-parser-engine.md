# ADR-003: Generic Parser Engine

- **Date:** 2026-05-10
- **Status:** Proposed
- **Deciders:** Architecture team + AI Agent (autonomous L4)
- **Depends on:** ADR-001 (registry), ADR-002 (matcher)

## Context

Post-Faza 3, `cmd/main.go::handleMessage` are un dispatcher pe `streamID` (din matcher), dar **logica de parsing per stream e încă inline** în main.go (~200 linii):

```go
case "telemetry":
    var sun struct { Ts string; Measurements []map[string]any; HouseLoad float64 }
    json.Unmarshal(payload, &sun)
    // ... extract loop, build fields, NewPoint, writePoint
case "state":
    var state StateMessage
    json.Unmarshal(payload, &state)
    relayOn := 0; if strings.EqualFold(state.POWER, "ON") { relayOn = 1 }
    // ... NewPoint, writePoint
case "sensor": ...
case "emeter": ...
case "relay": ...
case "zigbee": ...
```

Probleme:
- 5+ struct-uri vendor-specifice (`StateMessage`, `SensorMessage`, `EnergyData`) hardcoded în `cmd/main.go`
- Logică de parsing nu e testabilă în izolare (trebuie test integration cu MQTT message)
- Adăugare nou vendor = add new `case` + new struct → exact problema pe care Faza 4 o rezolvă

## Decision

Extragem parsarea într-un package separat `internal/parsers/` cu:

1. **Tip `ParsedTelemetry`** comun — output uniform pentru orice parser
2. **Funcție-dispatcher `parsers.Parse(streamID, topic, payload, dd, extracted)`** care rutează la parser-ul corect bazat pe `streamID`
3. **Parser per stream** (tasmota_state, tasmota_sensor, huawei_telemetry, shelly_emeter, shelly_relay, zigbee_json)
4. **Tests izolate** pentru fiecare parser cu samples reale

`cmd/main.go::handleMessage` devine:

```go
match := topicMatcher.Match(topic)
streamID := determineStream(match, parsed)

// Streams care NU produc Influx points (control plane)
switch streamID {
case "cmd_ack": handleCmdAck(payload, deviceID); return
case "ota":     handleOTA(payload, deviceID); return
case "shadow":  handleShadow(payload, deviceID); return
}

// Streams data → parser engine
result, err := parsers.Parse(streamID, topic, payload, match.Definition, match.Extracted)
if err != nil { logging.Drop(...); return }

p := influxdb2.NewPoint("devices",
    map[string]string{"device": deviceID, "source": result.Source, "type": result.Type, "tenant_id": tenantTag},
    result.Fields, result.Timestamp)
writePoint(p, pool, tenantPlan, ...)
```

### `ParsedTelemetry` schema

```go
type ParsedTelemetry struct {
    Timestamp time.Time              // pt SUN2000 vine din ts; altfel time.Now()
    Source    string                 // tag Influx: sun2000 / nousat / shelly / zigbee2mqtt
    Type      string                 // tag Influx: solar_inverter / state / energy / power_meter / relay / sensor
    Fields    map[string]interface{} // datele propriu-zise (gata pt Influx)
}
```

Fields preserve **vendor-named legacy keys** (`relay_state`, `nousat_power`, `pv_input_power`) ca să nu spargem dashboard-ul actual care le interogheaza direct.

### Parser registry

```go
type ParserFunc func(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error)

var streamParsers = map[string]ParserFunc{
    "telemetry": parseHuaweiTelemetry,
    "state":     parseTasmotaState,
    "sensor":    parseTasmotaSensor,
    "emeter":    parseShellyEmeter,
    "relay":     parseShellyRelay,
    "zigbee":    parseZigbeeJSON,
}

func Parse(streamID, topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
    fn, ok := streamParsers[streamID]
    if !ok {
        return parseGeneric(topic, payload)  // fallback: flat JSON, source=generic
    }
    return fn(topic, payload, dd, extracted)
}
```

### Field normalization (out of scope)

Roadmap §Faza 4 menționează "field normalization layer (vendor field → normalized name + unit conversion)". O amânăm pentru o sub-fază 4.5 sau Faza 5 pentru că:

- Dashboard-ul actual queryează vendor-named fields (`pv_input_power`, `nousat_power`)
- Schimbarea numelor fields = breaking change frontend → afară din scope-ul refactor pure ingest
- Faza 5 (Capability Engine) e momentul natural să introducem normalized vocabulary la nivel de capability

Pentru Faza 4 actuală: parsers produc EXACT aceleași field names cu cele actuale. Behaviour identic, doar code structure curat.

## Alternatives Considered

### A. Per-DD parser type
**Pro:** Toate stream-urile unui DD merge prin același parser config.
**Con:** Tasmota are STATE și SENSOR cu format complet diferit (POWER:str vs ENERGY:obj). Un singur parser nu acoperă ambele.
**Decizie:** Reject — parser per stream e mai natural.

### B. Reflection-based parser (struct tags)
**Pro:** Mai puțin cod boilerplate.
**Con:** Reflection lent + greu de debug; struct tags nu pot exprima "ON->1, OFF->0" transform.
**Decizie:** Reject — explicit code per parser e mai clar.

### C. Keep parsing inline în main.go cu funcții helper
**Pro:** Nu adăugăm package nou.
**Con:** main.go rămâne god-file; testabilitate slabă.
**Decizie:** Reject — separation of concerns mandatory.

## Consequences

### Positive
- **Testabilitate:** fiecare parser are unit tests cu 5–15 sample payloads reale
- **Extensibility:** parser nou pentru vendor nou = un fișier `internal/parsers/<vendor>.go` + register stream
- **main.go shrink:** ~200 linii de parsing inline → ~30 linii dispatcher
- **Coverage measurable:** parsers package poate fi covered > 90%

### Negative
- **Dispatch overhead:** map lookup per mesaj. Negligibil (< 100ns).
- **Boilerplate:** fiecare parser e ~30-50 linii. Acceptabil pt clarity.

### Neutral
- **Backwards compat:** field names păstrate (vendor names) → zero impact dashboard.

## Implementation Plan

### Faza 4 (acest ADR)

1. Create `internal/parsers/`:
   - `parser.go` — `ParsedTelemetry` + `ParserFunc` + dispatcher
   - `tasmota.go` — state + sensor parsers (StateMessage/SensorMessage moved here)
   - `huawei.go` — telemetry parser (with measurements array)
   - `shelly.go` — emeter + relay parsers
   - `zigbee.go` — Z2M JSON parser
   - `generic.go` — fallback flat JSON
2. `cmd/main.go` refactor:
   - Remove `StateMessage`, `SensorMessage`, `EnergyData` struct definitions
   - Remove inline JSON unmarshal logic per case
   - Replace with `parsers.Parse(streamID, topic, payload, dd, extracted)`
3. Tests `internal/parsers/parsers_test.go`:
   - 5+ samples per parser type din producție logs
   - Edge cases: malformed JSON, missing fields, unexpected types
   - Round-trip: payload → ParsedTelemetry → assert fields
4. Quality gates:
   - go vet, build, test full suite
   - Coverage `internal/parsers/` > 85%
   - Bench parser hot path < 10µs

### Out of scope (alte faze)

- Faza 4.5 / Faza 5: Field normalization layer (vendor → canonical + units)
- Faza 5: Capability engine consumă `dd.Capabilities`

## Validation

- Test coverage > 85% pe `internal/parsers/`
- Bench parser dispatcher < 10µs / op
- Smoke test: replay 100 mesaje MQTT din `logs/buffer.jsonl` → toate produc fields identice cu pre-Faza-4 (regression test)

## References

- ADR-001 (registry foundation)
- ADR-002 (matcher routing)
- Roadmap `docs/upgrade_md_iot_platform_refactor_ai_ready.md` §Faza 4
