# ADR-002: Generic Topic Matcher

- **Date:** 2026-05-10
- **Status:** Proposed
- **Deciders:** Architecture team + AI Agent (autonomous L4)
- **Depends on:** ADR-001 (registry)

## Context

Routarea mesajelor MQTT în `cmd/main.go` se face prin lanț de `if/else` cu `strings.Contains` / `strings.HasSuffix`:

```go
if strings.HasSuffix(topic, "/up/cmd_ack") || parsed.Stream == "cmd_ack" { ... }
else if strings.HasSuffix(topic, "/up/ota") { ... }
else if strings.HasSuffix(topic, "/up/shadow") { ... }
else if strings.Contains(topic, "/emeter/0/") { ... }
else if strings.Contains(topic, "/relay/0") { ... }
else if strings.HasSuffix(strings.ToLower(topic), "/state") { ... }
else if strings.HasSuffix(strings.ToLower(topic), "/sensor") { ... }
else if parsed.Stream == "telemetry" { ... }     // SUN2000
else if strings.HasPrefix(topic, "zigbee2mqtt/") { ... }
else { /* generic auto_detected */ }
```

Probleme:
- **Fragil:** la nou vendor → încă un `else if` în cod
- **Untestable:** routing implicit, nu declarativ
- **Vendor leak:** Shelly/Tasmota/Z2M conventions hardcoded în Go
- **Nu folosește Faza 2:** registry-ul YAML există dar nu controlează routing-ul

## Decision

Înlocuim lanțul de `strings.Contains/HasSuffix` cu un **Topic Matcher generic** care consumă registry-ul YAML din Faza 2.

### Algoritm

```
1. La startup: Matcher.New(registry) compile-uiește toate `topic_match` patterns din toate DD-urile
2. Per mesaj MQTT primit:
   a. matcher.Match(topic) → returnează (DD, stream, extracted vars) sau nil
   b. dispatch pe (DD.id, stream) către handler-ul corespunzător
3. La match nil: fallback la handler generic / log + drop
```

### Pattern syntax suportat

Două dialecte în câmpul `pattern:`:

**MQTT wildcards** (default — fără prefix):
- `+` matchează un singur segment (`[^/]+`)
- `#` matchează 0+ segmente; **trebuie ultim segment**
- Ex: `tenants/+/devices/+/up/state` → matches `tenants/2/devices/abc/up/state`
- Capture groups numerotate `$1`, `$2`, ... corespund pozițiilor `+` (în ordine)

**Regex literal** (prefix `~`):
- Tot ce urmează e Go regexp standard
- Suportă named groups `(?P<name>...)` referințate prin nume
- Ex: `~^/(?P<sn>\d+)/[^/]+/[^/]+/telemetry$` → captură `sn`

### Schema extension: `stream` per pattern

Fiecare `topic_match[i]` primește câmp opțional `stream:` care identifică tipul logic al mesajului (telemetry / state / sensor / cmd_ack / shadow / etc.). Folosit de dispatcher-ul în `cmd/main.go` ca să decidă handler-ul.

```yaml
topic_match:
  - pattern: "tele/+/STATE"
    stream: "state"
    extract: { device_id: "$1" }
  - pattern: "tele/+/SENSOR"
    stream: "sensor"
    extract: { device_id: "$1" }
```

### Performance

Target: matcher.Match() < 50µs p99 pentru 100 DD-uri × 5 patterns = 500 patterns.

Implementare:
- Patterns compile-uite la startup în `*regexp.Regexp` (nu re-compile la fiecare match)
- Iterație liniară (cache-friendly pentru < 1000 patterns)
- Pentru >1000 patterns: trie pe prefixe (out of scope pentru Faza 3)

### Backwards compatibility

Feature flag `MATCHER_ENABLED` (default `true`):
- `true` → folosește matcher pentru routing
- `false` → fallback la lanțul vechi `strings.Contains` (pentru rollback rapid în caz de bug)

Lanțul vechi rămâne în cod 4 săptămâni (post-merge), apoi e șters odată cu sign-off.

### Match priority

În caz că un topic matchează multiple patterns:
- **Ordine:** primul match câștigă
- **Order:** patterns sunt ordonate în registry după ordinea încărcării DD-urilor (alfabetic pe ID), apoi după ordinea în `topic_match[]` din YAML
- **Detectie ambiguitate:** la load, matcher loghează WARN dacă două patterns sunt structural identice (rejection ar fi prea agresiv)

## Alternatives Considered

### A. Trie-based matching
**Pro:** O(log N) lookup pentru large N.
**Con:** Complexitate ridicată; câștig negligibil până la 10K+ patterns.
**Decizie:** Linear scan acum, trie e Faza 3.5+ dacă scale impune.

### B. MQTT broker-side routing (EMQX rules)
**Pro:** Native MQTT, no Go code.
**Con:** Logica decuplată de cod, harder to test, vendor lock-in pe EMQX.
**Decizie:** Reject — keep routing în Go pentru testabilitate.

### C. Pure regex (fără MQTT wildcards)
**Pro:** Simpler implementation.
**Con:** YAML-uri verbose; ops/integrators trebuie să cunoască regex-uri.
**Decizie:** Suportăm ambele — MQTT wildcards prietenoase + regex când e nevoie.

## Consequences

### Positive
- **Onboarding device nou:** doar YAML, zero cod Go
- **Testabilitate:** matcher are 30+ teste; routing decisions explicite
- **Audit trail:** routing decisions logged cu DD-id + extracted vars
- **Decoupling:** ingest pipeline nu mai cunoaște vendori specifici

### Negative
- **Complexitate la startup:** patterns compile (≈ 100ms pentru 100 DD-uri — neglijabil)
- **Migration risk:** dacă matcher are bug pe edge case, ingest cade. Mitigare: feature flag + dual-path testing 2 săpt.
- **Regex în YAML:** users pot scrie regex incorecte care compilează dar matchează greșit. Mitigare: validator la load + tests pe production configs.

### Neutral
- **Memory:** ~1KB per pattern compiled = ~500KB pentru 500 patterns. Negligibil.

## Implementation Plan

### Faza 3 (acest ADR)

1. Extend `TopicMatchSpec` cu `Stream string` (in registry/types.go)
2. Update YAMLs existente (`configs/devices/*.yaml`) cu `stream:` per pattern
3. New package `internal/matcher/` cu:
   - `Matcher` struct
   - `New(*registry.Registry) (*Matcher, error)`
   - `Match(topic string) *Match`
   - MQTT wildcard → regex compiler (`mqttToRegex`)
   - Extraction logic (`$N` positional + named groups)
4. Test suite:
   - 30+ test cases pe pattern matching
   - Benchmark `BenchmarkMatcher` cu target < 50µs/op
   - Integration test cu configs production
5. Integrate în `cmd/main.go`:
   - Load registry + create matcher la startup
   - Add `MATCHER_ENABLED` env flag (default: true)
   - Run matcher per message; dispatch pe (dd, stream)
   - Old `strings.Contains` chain kept ca fallback dacă MATCHER_ENABLED=false
6. Quality gates:
   - go vet, build, test pe toate pachetele
   - bench < 50µs
   - end-to-end smoke test cu MQTT replay

### Out of scope (alte faze)

- Faza 4: Parser engine consumă (DD, stream) pentru parsing-ul payload-ului
- Faza 5: Capability engine consumă DD.Capabilities pentru frontend filtering

## Validation

- Test coverage > 85% pe `internal/matcher/`
- Benchmark `BenchmarkMatcher` < 50µs/op asserted în CI
- Smoke test: 100 mesaje MQTT replay (din `logs/buffer.jsonl`) → toate routate corect

## References

- ADR-001 (registry foundation)
- Roadmap `docs/upgrade_md_iot_platform_refactor_ai_ready.md` §Faza 3
- MQTT 5.0 §4.7 Topic Names and Topic Filters
