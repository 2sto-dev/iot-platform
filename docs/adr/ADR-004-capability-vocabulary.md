# ADR-004: Capability Vocabulary & Engine

- **Date:** 2026-05-10
- **Status:** Proposed
- **Deciders:** Architecture team + AI Agent (autonomous L4)
- **Depends on:** ADR-001 (registry), ADR-002 (matcher), ADR-003 (parser engine)

## Context

Frontend-ul (React) și rules engine fac DECIZII bazate pe `device_type`:

```ts
// dashboard/src/components/Layout.tsx
{ to: "/solar", label: "Solar", requireDevice: "sun2000" }
{ to: "/boiler", label: "Boiler", requireDevice: "nous_at" }

// dashboard/src/pages/SolarPage.tsx
const sun2000 = devices.filter((d) => d.device_type === "sun2000");

// dashboard/src/pages/BoilerPage.tsx
const nousat = devices.filter((d) => d.device_type === "nous_at");
```

Probleme:
- Vendor lock — adăugăm **Deye SUN5K-G2**? Trebuie modificare frontend cu nou `device_type === "deye"`. Dar e FUNCȚIONAL un inverter — ar trebui să apară pe Solar page out-of-the-box.
- Multi-purpose devices — Nous A1T e **smart_plug**, dar ESTE și relay + power_meter. Frontend-ul ar trebui să arate butonul ON/OFF (capability=relay) DAR și gauge consum (capability=power_meter), independent de vendor.
- Rules engine la fel — o regulă "alertă temperatură batterie > 50°C" trebuie să meargă pe ORICE device cu capability=`battery`, nu doar SUN2000.

## Decision

Introducem **Capability Engine** care:

1. Definește un **vocabular canonical** de capabilities standardizate (relay, inverter, battery, power_meter, etc.)
2. Suportă **inheritance** între capabilities (ex: `smart_plug` ⇒ `relay` + `power_meter`)
3. Denormalizează capabilities din DD în Django `Device.capabilities` (JSONField)
4. Frontend interogheaza pe capabilities, NU pe `device_type`
5. API expune filter: `GET /api/devices/?capability=inverter`

### Canonical Vocabulary (v1)

Capabilities sunt împărțite în **categorii**:

#### Energy
- `inverter` — convertor DC↔AC, are `pv_input_power`, `active_power`, etc.
- `battery` — sistem stocare, are `battery_soc`, `battery_temp`, `battery_power`
- `solar_pv` — string/array fotovoltaic, are `pv_voltage`, `pv_current`
- `power_meter` — măsoară consum/producție (single-phase sau 3-phase)
- `smart_meter` — sub-tip de power_meter cu `total_imported`, `total_exported`

#### Switching
- `relay` — ON/OFF singular, are `relay_state` / `relay_on`
- `dimmer` — control variabil (0-100%) pentru lumini
- `light` — sub-tip dimmer + color management
- `cover` — blinds / shutters cu poziție 0-100%
- `valve` — water/gas valve ON/OFF cu fail-safe

#### Sensing
- `temperature_sensor` — temp °C
- `humidity_sensor` — RH %
- `motion_sensor` — PIR detection (boolean)
- `door_sensor` — open/closed
- `pressure_sensor` — hPa / bar
- `co2_sensor` — ppm
- `lux_sensor` — lumens

#### Composite (cu inheritance)
- `smart_plug` ⇒ `relay` + `power_meter` (Nous A1T, Shelly Plus 1PM)
- `hybrid_inverter` ⇒ `inverter` + `battery` (Huawei SUN2000 + LUNA2000)
- `climate_sensor` ⇒ `temperature_sensor` + `humidity_sensor` (Aqara, Sonoff)
- `eb_charger` ⇒ `relay` + `power_meter` (planificat — EV chargers)

#### Connectivity (meta)
- `battery_powered` — folosește baterie internă (afectează polling rate)
- `mains_powered` — alimentat din rețea
- `wifi` / `zigbee` / `lora` / `modbus_tcp` — protocol fizic

### Inheritance Engine

Algoritm `Resolve(declared []string) []string`:

```
1. queue = declared
2. seen = {}
3. while queue non-empty:
     c = pop(queue)
     if c in seen: continue
     seen.add(c)
     parents = inheritanceMap[c]
     queue.extend(parents)
4. return list(seen)
```

Exemplu:
- DD declares: `[smart_plug]`
- After resolve: `[smart_plug, relay, power_meter]`

Ciclurile sunt blocate (seen set). Validator runtime detectează inheritance loop la load.

### Schema YAML

DD-urile rămân declarative:

```yaml
capabilities:
  - smart_plug              # high-level (gets relay + power_meter automatic)
  - mains_powered           # meta
  - wifi
```

Loader-ul Go aplică `Resolve()` și expune capabilities expandate via `dd.ResolvedCapabilities`. Pentru back-compat, `dd.Capabilities` rămâne lista declared (raw); resolved e un câmp computed.

### Django side

`Device.capabilities` e `JSONField(default=list)`. Populat automat:
- La creare device: signal `pre_save` lookup în Python YAML loader (cache module-level)
- Migration backfill: management command `sync_capabilities_from_dd`

Filter API:
```
GET /api/devices/?capability=inverter
```
Implementat ca `Device.objects.filter(capabilities__contains=[capability_name])`. MySQL 8+ suportă `JSON_CONTAINS()` nativ.

### Frontend side

Tipul `Device` în TypeScript primește `capabilities: string[]`.

Sidebar `Layout.tsx` filtrează pe capabilities:
```ts
const allLinks = [
  { to: "/devices", label: "Devices", requireCapability: null },
  { to: "/solar", label: "Solar", requireCapability: "inverter" },
  { to: "/boiler", label: "Boiler", requireCapability: "smart_plug" }, // sau "relay"
  ...
];

const links = allLinks.filter(l =>
  !l.requireCapability ||
  devices.some(d => d.capabilities.includes(l.requireCapability))
);
```

Dashboard pages query `/api/devices/?capability=X` în loc de filter local.

## Alternatives Considered

### A. Capabilities ca string single (`device_kind: "smart_plug"`)
**Pro:** Simpler.
**Con:** Pierd composition. Un device poate avea multiple capabilities (smart_plug = relay + power_meter).
**Decizie:** Reject.

### B. Capabilities derivate la query time (no denormalization)
**Pro:** Single source of truth (YAML doar).
**Con:** Frontend trebuie să ceară DD-ul fiecare device → N+1 query problem; rules engine la fel.
**Decizie:** Reject — denormalizare în DB.

### C. Per-capability tabela M2M Django
**Pro:** Normalized SQL, fast filter.
**Con:** Capability list se schimbă rar → JSONField + index e suficient. M2M adds complexitate (intermediate table, joins).
**Decizie:** JSONField + MySQL JSON_CONTAINS.

### D. Hierarchical class-based (OO inheritance)
**Pro:** Familiar OOP pattern.
**Con:** Capabilities NU sunt clase Go; inheritance e despre **trait composition**, nu sub-typing.
**Decizie:** Map-based inheritance.

## Consequences

### Positive
- **Vendor onboarding:** Deye/GoodWe/SolarEdge inverter = YAML cu `capabilities: [inverter, battery]` → apare automat pe Solar page
- **Semantic queries:** "all devices with battery temp" = filter capability=battery, nu cunosc vendor-uri
- **Rules engine:** o regulă "battery_soc < 20%" pe capability=battery, aplică TOATE bateriile
- **Frontend coherent:** sidebar/pages filtrează semantic

### Negative
- **Vocabulary governance:** lista canonical trebuie întreținută. Adăugare nouă = ADR + review (proces).
- **Backwards compat:** `device_type` rămâne în model (nu break-uim DEVICE_CHOICES), doar adăugăm capabilities.
- **Sync overhead:** Django + Go trebuie să citească aceleași YAML-uri. Mitigare: ambele citesc `configs/devices/` direct.

### Neutral
- **Storage:** capabilities ~200 bytes per device în JSONField. Negligibil.

## Implementation Plan

### Faza 5 (acest ADR)

1. Go `internal/capabilities/`:
   - `vocabulary.go` — constante canonical + descriptions
   - `inheritance.go` — `inheritanceMap` + `Resolve(declared []string) []string`
   - `validator.go` — verifică DD.Capabilities sunt în vocabulary la load
   - Tests: vocabulary completness, inheritance Resolve, cycle detection
2. Update YAMLs: replace ad-hoc cu canonical names + inheritance `smart_plug`
3. Update `internal/registry/types.go`: `DeviceDefinition.ResolvedCapabilities` populat la load
4. Django:
   - Migration `0011_device_capabilities` cu `JSONField(default=list)`
   - `clients/dd_loader.py` — citește YAML files, cache dict device_type → [capabilities]
   - `pre_save` signal pe Device să populeze `capabilities`
   - Management command `sync_capabilities` backfill
   - `DeviceSerializer.fields += ['capabilities']`
   - `DeviceViewSet.get_queryset`: filter `?capability=X`
5. Frontend:
   - `Device` interface: `capabilities: string[]`
   - `Layout.tsx`: refactor `requireDevice` → `requireCapability`
   - `SolarPage`/`BoilerPage`: `filter(d => d.capabilities.includes(...))`

### Out of scope (alte faze)

- Faza 6 (Runtime): runtime capabilities (online/offline, last_seen) — separat de DD-declared capabilities
- Vocabulary expansion: noi capabilities adăugate prin ADR-uri viitoare (-005, -006...)

## Validation

- Test coverage > 85% pe `internal/capabilities/`
- Django tests: `Device.capabilities` populat corect post-create
- E2E: dashboard sidebar filtrează corect; `/api/devices/?capability=inverter` răspunde corect
- Smoke test: `manage.py sync_capabilities` populează existing devices fără gap

## References

- ADR-001/002/003 (registry, matcher, parser)
- Roadmap §Faza 5
- Home Assistant device_class (similar concept): https://www.home-assistant.io/integrations/sensor/#device-class
- Matter/Thread cluster spec (industry inspiration): https://csa-iot.org/
