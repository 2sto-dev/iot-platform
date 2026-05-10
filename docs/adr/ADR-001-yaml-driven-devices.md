# ADR-001: YAML-driven Device Definitions

- **Date:** 2026-05-10
- **Status:** Proposed
- **Deciders:** Architecture team + AI Agent (autonomous L3)
- **Supersedes:** —
- **Superseded by:** —

## Context

Platforma IoT actuală are logica device-specific **hardcoded** în multiple locuri:

- `go-iot-platform/cmd/main.go` — 7+ ramuri `strings.Contains` / `HasSuffix` pe topic + 5+ `parsed.Stream == "X"` switch-uri
- `go-iot-platform/cmd/main.go` — 5 struct-uri Go specifice payload-ului vendor (`StateMessage`, `SensorMessage`, `EnergyData`, etc.)
- `django-bakend/clients/models.py` — `DEVICE_CHOICES = [(...)]` enumerare statică (5 valori curent)
- `dashboard/src/components/solar/InverterPanel.tsx` — 50+ field names Huawei hardcoded în `defaultInverterGroups`
- `dashboard/src/components/solar/BatteryPanel.tsx` — similar

Pentru fiecare nou vendor (ex: Deye, GoodWe, ESPHome custom):
1. Modificare cod Go (handler nou)
2. Adăugare în `DEVICE_CHOICES` Django
3. Migrare DB
4. Update toate dashboard panel-urile
5. Re-deploy întregul stack

**Problema:** scaling la 100+ device types e neviabil. Fiecare nou vendor = 1 sprint.

## Decision

Adoptăm **YAML-driven device definitions** ca single source of truth pentru:

1. **Identification** — cum recunoaștem un device dintr-un topic MQTT (regex/wildcard)
2. **Parser config** — cum extragem fields dintr-un payload (json / measurements_array / raw / keyvalue)
3. **Field normalization** — mapping vendor field → canonical name (`pv_input_power` → `solar_power_kw`)
4. **Capabilities** — taguri abstracte (`inverter`, `battery`, `relay`, `power_meter`)
5. **Commands** — topice și payload-uri pentru downlink
6. **Telemetry streams metadata** — interval, retention hint

YAML files sunt în `configs/devices/*.yaml`, versionate Git, încărcate de Go la startup și cache-uite în memorie.

### Schema versioning

Header `schema_version: "1.0"` în fiecare fișier. Loader-ul rejectează versiuni necunoscute. Migrări viitoare prin script `migrate_dd.go` care rulează pe directorul `configs/devices/`.

### Validare

Loader-ul aplică validări:
- Required fields: `id`, `name`, `protocol`, `identification`, `parser`, `capabilities`
- `id` unic în registru (duplicat → load fail-fast)
- `protocol` ∈ {`mqtt`, `modbus_tcp`, `http`, `coap`}
- `identification.topic_match[*].pattern` regex-valid
- `parser.type` ∈ {`json`, `json_with_measurements_array`, `raw`, `keyvalue`}
- `capabilities[*]` ∈ vocabular canonical (definit în `internal/capabilities/vocabulary.go`)

### Hot-reload (Faza 2 — out of scope, planificat Faza 3)

Faza 2 încarcă DD doar la startup. Hot-reload via SIGHUP sau fsnotify e extensie pentru Faza 3 după ce matcher-ul e implementat.

## Alternatives Considered

### A. JSON Schema în loc de YAML

**Pro:** Validare nativă cu drf-spectacular / `gojsonschema`, conversie ușoară spre OpenAPI.
**Con:** Mai puțin lizibil pentru ops/integrators (verbose, fără comentarii inline). YAML câștigă pe DX.
**Decizie:** YAML pt sursă, JSON Schema pt validation contract (`configs/devices/_schema.json`).

### B. TOML

**Pro:** Simplu.
**Con:** Mai puțin nested-friendly. YAML e standard în Kubernetes / Ansible / GitOps — community alignment.
**Decizie:** Reject.

### C. Database (MySQL) cu admin UI

**Pro:** Editare runtime fără redeploy.
**Con:** Pierderea Git versioning, code review pe schimbări, rollback simplu. Crește complexitatea (migrations, race conditions cu cache).
**Decizie:** YAML files cu Git GitOps (PR review pentru orice DD nou) e mai aliniat cu enterprise practices.

### D. Cue (Configure Unify Execute)

**Pro:** Type-safe, schema embedded, mai puternic decât JSON Schema.
**Con:** Curva învățare, ecosystem mai mic, tooling Go matur dar mai puțin documentat.
**Decizie:** Evaluare pentru v2.0 al schemei după learnings din Faza 2-4. Acum **YAML + gojsonschema**.

## Consequences

### Positive

- **Onboarding device nou** = 1 fișier YAML + 1 PR review, zero cod nou (când schema e suficientă)
- **Audit trail** prin Git — orice schimbare e PR review-ată
- **Testabilitate** — DD e date, ușor de mock-uit
- **Documentație implicită** — YAML e self-documenting cu comments
- **Frontend decoupling** — UI cere "all devices with capability=inverter", nu "device_type=sun2000"

### Negative

- **Performance overhead** la startup: parse N fișiere YAML. Mitigare: cache în memorie + lazy validation.
- **Validare runtime mandatory** — un YAML invalid poate cauza panic dacă nu validăm strict. Mitigare: fail-fast la startup + tests pe malformed YAML.
- **Schema evolution risk** — schimbări breaking în schema_version necesită migrare DD-uri. Mitigare: versioning explicit + script migrate.
- **Tooling debt** — necesită yamllint în CI + JSON Schema validator runtime.

### Neutral

- **Curve dezvoltatori** — minimă; YAML e standard.
- **Storage** — DD-urile sunt < 5KB / device, total < 1MB pentru 200 vendors → negligibil.

## Implementation Plan

### Faza 2 (acest ADR)

1. `configs/devices/` cu 4 exemple (Huawei SUN2000, Nous A1T, Shelly EM, Zigbee temp)
2. `internal/registry/types.go` — `DeviceDefinition` struct
3. `internal/registry/loader.go` — `LoadDeviceDefinitions(dir)` + cache
4. `internal/registry/validator.go` — strict validation
5. Unit tests cu valid + malformed YAML
6. Integration la startup `cmd/main.go` (registry e disponibil în context, dar NEFOLOSIT încă pentru ingest — Faza 3)

### Out of scope (alte faze)

- Faza 3: Topic Matcher consumă DD pentru routing (înlocuiește hardcoded `strings.Contains`)
- Faza 4: Parser engine consumă DD `parser:` config (înlocuiește struct-urile hardcoded)
- Faza 5: Capability engine consumă DD `capabilities:` block

## Validation

- Unit tests acoperă: load valid YAML, reject malformed YAML, reject duplicate IDs, reject unknown schema_version, reject invalid regex în identification.
- Coverage target: > 85% pe `internal/registry/`.
- Integration test: la startup `cmd/main.go`, log line `loaded N device definitions` cu N ≥ 4.

## References

- Original upgrade roadmap: [`docs/upgrade_md_iot_platform_refactor_ai_ready.md`](../upgrade_md_iot_platform_refactor_ai_ready.md) §Faza 2
- Schema example în roadmap §Target Architecture
- ABNF MQTT topics: RFC 5234 + MQTT 5.0 spec §4.7
