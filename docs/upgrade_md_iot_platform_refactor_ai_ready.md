# IoT Platform — Enterprise Upgrade Roadmap
## YAML Device Architecture · Generic Runtime · Multi-Tenant Refactor · SRE-Ready

> **Versiune document:** 2.0 (Enterprise edition)
> **Status:** Living document — actualizat la finalul fiecărei faze
> **Audiență:** Arhitecți, dev leads, SRE, security, product owners, AI agents
> **Document owners:** Architecture team + AI Agent (Claude Code)

---

## CUPRINS

1. [Executive Summary](#executive-summary)
2. [Glossary & Acronyms](#glossary--acronyms)
3. [Prerequisites Verification](#prerequisites-verification) ← **citește prima dată**
4. [Current State Assessment](#current-state-assessment)
5. [Target Architecture & Non-Functional Requirements](#target-architecture--non-functional-requirements)
6. [Phase Plan (Faza 1–9)](#phase-plan)
7. [Cross-Cutting Concerns](#cross-cutting-concerns)
8. [Risk Register](#risk-register)
9. [Effort & Capacity Estimation](#effort--capacity-estimation)
10. [Tooling Matrix](#tooling-matrix)
11. [RACI & Approval Matrix](#raci--approval-matrix)
12. [Quality Gates per Phase](#quality-gates-per-phase)
13. [CI/CD Pipeline Requirements](#cicd-pipeline-requirements)
14. [Disaster Recovery & Backup](#disaster-recovery--backup)
15. [Compliance & Security Checklist](#compliance--security-checklist)
16. [Capacity Planning & Cost Model](#capacity-planning--cost-model)
17. [Observability Stack](#observability-stack)
18. [AI Agent Development Conditions](#ai-agent-development-conditions)
19. [Definition of Done — Enterprise](#definition-of-done--enterprise)
20. [Documentation Standards](#documentation-standards)
21. [Appendix A — Reference Implementations](#appendix-a)

---

## Executive Summary

Platforma IoT actuală (Django + Go + MQTT/EMQX + InfluxDB + MySQL + Redis + Kong + React) deservește **multi-tenant** la scară mică (3 tenants, ~10 device-uri active) cu logică **vendor-specific hardcodată** în Go ingest (handlers per `topic suffix`) și Django (`DEVICE_CHOICES` enumerare statică).

Această roadmap transformă platforma în:

- **YAML-driven device definitions** — onboarding de noi vendors fără cod nou
- **Capability-based abstraction** — frontend & rules nu mai depind de vendor/protocol
- **Generic parser engine** — eliminare `StateMessage`/`SensorMessage` hardcoded
- **Runtime state engine** — observabilitate device cu online/offline/last_seen
- **Command engine multi-protocol** (MQTT/Modbus/HTTP)
- **Enterprise multi-tenant isolation** — SOC 2 / GDPR / ISO 27001 ready

**Ținta scale:** 5,000 tenants × 20,000 device-uri × 10M mesaje MQTT/zi.

**Effort estimat:** 380–520 ore dev (≈ 10–13 săptămâni cu 1 FTE Senior + AI agent assist).

**Risc principal:** breaking changes pe ingest pipeline existent → **mitigare: feature flag `LEGACY_INGEST_ENABLED` + dual-write 4 săptămâni**.

---

## Glossary & Acronyms

| Termen | Definiție |
|---|---|
| **Capability** | Caracteristică funcțională a unui device (ex: `inverter`, `relay`, `temperature_sensor`). Independentă de vendor. |
| **Device Definition (DD)** | Fișier YAML care descrie un model de device: protocol, topice, fields, parser, capabilities, comenzi. |
| **Topic Matcher** | Engine care leagă un topic MQTT primit de un Device Definition (wildcard / regex). |
| **Normalized Field** | Câmp standardizat platform-wide (ex: `power_w`) care abstractizează numele vendor (ex: `pv_input_power`, `apparent_power`). |
| **Runtime** | Stare in-memory a unui device (last_seen, online status, last telemetry). |
| **Driver / Adapter** | Cod vendor-specific (translator legacy → schema generală). |
| **SLO** | Service Level Objective — target intern (ex: 99.5% uptime). |
| **SLA** | Service Level Agreement — angajament contractual cu clientul. |
| **RPO** | Recovery Point Objective — pierdere maximă de date acceptată (ex: 5 min). |
| **RTO** | Recovery Time Objective — timp maxim de restaurare (ex: 30 min). |
| **TSI** | Time Series Index (InfluxDB) — afectează field type locks. |

---

## Prerequisites Verification

> **Această secțiune trebuie validată COMPLET înainte de începerea Fazei 1.**
> Verificarea se face prin checklist; orice ❌ blochează startul.

### A. Infrastructură (must-have)

| Componentă | Versiune minimă | Status actual | Acțiune |
|---|---|---|---|
| **MySQL** | 8.0+ (sau MariaDB 10.6+) | ✅ Prezent (`mysqlclient==2.2.7`) | — |
| **PostgreSQL backup option** | 14+ | ⚠️ Recomandat pentru aggregates | Opțional Faza 8+ |
| **InfluxDB OSS** | 2.7+ | ✅ Prezent (3 buckets per plan) | — |
| **Redis** | 7.0+ | ✅ Prezent | — |
| **EMQX** | 5.x | ✅ Prezent | — |
| **Kong API Gateway** | 3.x DB-less | ✅ Prezent (alt VM) | — |
| **Go runtime** | 1.21+ | ✅ Prezent | — |
| **Python** | 3.12+ | ✅ Prezent | — |
| **Node.js (dev)** | 20 LTS+ | ✅ Prezent (Vite 7) | — |

### B. Cloud / VPS Resources

| Resursă | Min. necesar | Recomandat (5K tenants) |
|---|---|---|
| **App VM (Django+Go)** | 4 vCPU, 8 GB RAM, 50 GB SSD | 8 vCPU, 16 GB RAM, NVMe |
| **EMQX VM** | 2 vCPU, 4 GB RAM | 4 vCPU, 8 GB RAM, low latency network |
| **InfluxDB VM** | 4 vCPU, 16 GB RAM, 200 GB SSD | 8 vCPU, 64 GB RAM, NVMe RAID 1 |
| **Redis VM** | 2 vCPU, 4 GB RAM | 4 vCPU, 8 GB RAM, persistence enabled |
| **MySQL VM** | 2 vCPU, 4 GB RAM, 50 GB | 4 vCPU, 16 GB RAM, replica + backups |
| **Kong VM** | 1 vCPU, 2 GB RAM | 2 vCPU, 4 GB RAM, HA pair |

**Total enterprise sizing:** ~22 vCPU / 56 GB RAM / 350 GB SSD (excl. backups).

### C. Network & Connectivity

- [ ] Conectivitate **MQTT TLS** (port 8883) între EMQX și Go ingest
- [ ] Conectivitate **HTTPS** între Kong și Django (validate JWT)
- [ ] **VPC privat** sau VLAN izolat între componente (zero exposure direct la internet pentru DB)
- [ ] **Firewall rules** documentate: doar Kong are public IP
- [ ] **DNS intern** sau service discovery (Consul/etcd opțional)
- [ ] **Latency ms** între componente: < 5ms intra-VPC, < 100ms WAN client→Kong

### D. Tooling — Development

| Tool | Versiune | Required For | Status |
|---|---|---|---|
| **Git** | 2.40+ | All phases | ✅ |
| **Make** sau **Task** | latest | Build automation (lipsește în proiect actual) | ❌ **De adăugat** |
| **golangci-lint** | 1.55+ | Faza 3+ (linting Go) | ❌ **De adăugat** |
| **ruff** sau **flake8 + black** | latest | Python linting | ❌ **De adăugat** |
| **mypy** | 1.7+ | Type checking Python | ⚠️ Opțional |
| **pre-commit** hooks | 3.5+ | Quality gate pre-commit | ❌ **De adăugat** |
| **Docker** + **docker-compose** | 24.x | Local dev environment | ⚠️ Necesar Faza 9 |
| **Helm / Kubernetes** | 1.28+ | Production orchestration (când scale > 10K devices) | ❌ **De adăugat la Faza 10** |

### E. Tooling — Observability (Faza 1+ obligatoriu)

| Tool | Rol | Status |
|---|---|---|
| **Prometheus** | Metrics scrape | ⚠️ Configurat în Kong plugin doar — extinde pe Go binaries și Django |
| **Grafana** | Dashboards | ❌ **De adăugat** |
| **Loki** sau **ELK** | Centralized logs | ❌ **De adăugat** |
| **OpenTelemetry SDK** | Distributed tracing | ❌ **De adăugat** (Faza 6) |
| **Sentry** sau **Errbit** | Error tracking | ❌ **De adăugat** (Faza 5) |
| **Uptime monitoring** | Pingdom / UptimeRobot / self-hosted | ❌ **De adăugat** |

### F. Tooling — CI/CD (de implementat înainte de Faza 2)

| Componentă | Recomandare | Status |
|---|---|---|
| **CI runner** | GitHub Actions (există `.github/workflows/ci.yml`) | ✅ Bazic |
| **Pipeline gates** | golangci-lint + go test + pytest + tsc + npm audit + trivy scan | ⚠️ Parțial |
| **Deployment automation** | Terraform / Ansible / Pulumi | ❌ **De adăugat** |
| **Secret manager** | Vault / AWS Secrets Manager / Doppler | ❌ **De adăugat** (acum sunt în .env) |
| **Container registry** | GHCR / Docker Hub / ECR | ❌ **De adăugat** când deploy via Docker |

### G. Backup & Disaster Recovery

| Componentă | Frecvență backup | Retention | Status |
|---|---|---|---|
| **MySQL** | Daily full + hourly binlog | 30 days + 1y monthly | ❌ **De configurat** |
| **InfluxDB** | Daily snapshot per bucket | 90 days (per plan retention) | ❌ **De configurat** |
| **Redis** | RDB snapshot 6h + AOF | 7 days | ❌ **De verificat** |
| **YAML configs** (Faza 2+) | Git-versioned | Forever (in repo) | ✅ Auto |
| **Off-site backup** | S3 / B2 / encrypted offsite | 1 year | ❌ **De configurat** |
| **Restore drill** | Quarterly test restore | — | ❌ **De planificat** |

### H. Skills Prerequisites — Echipa & AI Agent

| Skill | Required level | Available? |
|---|---|---|
| **Go (concurrency, MQTT, Influx client)** | Senior | ✅ Existing codebase quality |
| **Django + DRF + signals** | Senior | ✅ |
| **React + TypeScript + TanStack Query** | Mid+ | ✅ |
| **InfluxDB Flux query language** | Mid+ | ⚠️ Există patterns; trebuie consolidare |
| **MQTT 5.0 protocol nuances** | Mid+ | ⚠️ Cunoaștere parțială (shared subs OK) |
| **YAML schema design** (JSON Schema / Cue) | Mid+ | ❌ De însușit pentru Faza 2 |
| **Modbus TCP** (pentru drivere SUN2000 etc) | Mid | ❌ De însușit Faza 7 |
| **SRE practices** (SLI/SLO, incident response) | Senior | ⚠️ Lipsește runbook |
| **Security: OWASP, JWT pitfalls, mTLS** | Senior | ⚠️ Lipsește audit |

### I. Compliance Prerequisites (B2B / Enterprise customers)

- [ ] **GDPR DPIA** completat (Data Protection Impact Assessment)
- [ ] **Data Processing Agreement (DPA)** template pentru clienți
- [ ] **Sub-processor list** publicată
- [ ] **Privacy policy** cu IoT-specific clauses (sensor data, retention)
- [ ] **Right to erasure** workflow documentat (per tenant + per device)
- [ ] **SOC 2 Type II** readiness assessment (dacă target = US enterprise)
- [ ] **ISO 27001 gap analysis** (dacă target = EU enterprise + critical infra)
- [ ] **Penetration testing** plan (anual)

### J. Repository & Process

- [ ] **Branching strategy** documentat (`main` / `develop` / `feature/*`)
- [ ] **CODEOWNERS** file pentru review obligatoriu
- [ ] **Conventional Commits** sau Angular commit format
- [ ] **CHANGELOG.md** auto-generat din commits
- [ ] **Architecture Decision Records (ADR)** folder + template
- [ ] **CONTRIBUTING.md** cu setup dev local
- [ ] **SECURITY.md** cu vuln disclosure process
- [ ] **CODE_OF_CONDUCT.md**

### Prerequisites Sign-Off

```
[ ] A. Infrastructură verificată — Owner: SRE Lead — Date: ____
[ ] B. Cloud sizing aprobat — Owner: CTO — Date: ____
[ ] C. Network & firewall configurate — Owner: SRE Lead — Date: ____
[ ] D. Tooling dev instalat — Owner: Tech Lead — Date: ____
[ ] E. Observability stack ready — Owner: SRE — Date: ____
[ ] F. CI/CD pipeline live — Owner: DevOps — Date: ____
[ ] G. Backup & DR tested — Owner: SRE — Date: ____
[ ] H. Team training plan — Owner: Eng Manager — Date: ____
[ ] I. Compliance baseline — Owner: Legal/CISO — Date: ____
[ ] J. Repo process aligned — Owner: Tech Lead — Date: ____
```

**Toate cele 10 categorii trebuie verificate ✅ înainte de a începe Faza 1 audit.** Categoriile cu ❌ în starea actuală necesită planificare separată (vezi §[Effort](#effort--capacity-estimation)).

---

## Current State Assessment

> **Sursă:** [README.md](../README.md) + [raport.md](../raport.md) + audit code 2026-05.

### Faza 0–4 (deja livrate înainte de upgrade)

| Faza | Status | Relevant pentru upgrade |
|---|---|---|
| **Faza 0** Stabilizare | ✅ Done | Foundation OK |
| **Faza 1** Multi-tenant refactor | ✅ Done | Faza 9 din upgrade plan = parțial deja livrată |
| **Faza 1.9** Hardening | ✅ Done | Strict tenant filter Influx + DeviceViewSet leak fix |
| **Faza 2** Ingest scalabil (EMQX, Redis cache, batch writes) | ✅ Done | Foundation OK pentru Faza 3 upgrade (matcher) |
| **Faza 3** Control plane (credentials, activation, commands, shadow, OTA) | ✅ Done | Faza 7 upgrade (command engine) parțial existent ca `cmd/downlink-worker` |
| **Faza 4** Rules + Notifications + Audit + API Keys | ✅ Done | Faza 8 upgrade (rule engine) parțial existent ca `cmd/rule-engine` |
| **Faza 5** React Dashboard | ✅ Done | Capability-driven UI necesar (Faza 5 upgrade) |

### Hardcoded patterns identificate (audit preliminar)

Confirmat prin grep pe codebase:

```
go-iot-platform/cmd/main.go:
  - strings.HasSuffix(topic, "/up/cmd_ack")
  - strings.HasSuffix(topic, "/up/ota")
  - strings.HasSuffix(topic, "/up/shadow")
  - strings.Contains(topic, "/emeter/0/")
  - strings.Contains(topic, "/relay/0")
  - strings.HasSuffix(strings.ToLower(topic), "/state")
  - strings.HasSuffix(strings.ToLower(topic), "/sensor")
  - parsed.Stream == "telemetry" → SUN2000 hardcoded handler
  + 5 type-specific structs (StateMessage, SensorMessage, EnergyData, etc.)

django-bakend/clients/models.py:
  - DEVICE_CHOICES = [...] static enum cu 5 valori (shelly_em / nous_at /
    zigbee_sensor / auto_detected / sun2000)

dashboard/src/components/solar/InverterPanel.tsx:
  - defaultInverterGroups hardcoded cu 50+ field names Huawei-specific

dashboard/src/components/solar/BatteryPanel.tsx:
  - defaultBatteryGroups hardcoded
```

**Concluzie audit:** Faza 1 din upgrade plan (audit) e **deja făcut prin acest document** — putem trece la Faza 2 după sign-off prerequisites.

### Gaps remaining (confirmate)

- ❌ Niciun **YAML device definition** — totul în cod
- ❌ Niciun **topic matcher** generic (toate `strings.Contains`)
- ❌ Niciun **parser engine** abstract (5+ structs hardcoded)
- ❌ **Capabilities** lipsesc complet (frontend depinde de `device_type`)
- ❌ **Runtime engine** lipsește (`last_seen`, online/offline tracking)
- ⚠️ **Command engine** parțial (Faza 3.3 generic, dar fără YAML mapping)
- ⚠️ **Rule engine** funcțional dar field-names hardcoded per device

---

## Target Architecture & Non-Functional Requirements

### Service Level Objectives (SLO)

| Service | SLI | SLO Target | SLA Comm. (B2B) |
|---|---|---|---|
| Ingest pipeline | Mesaje MQTT procesate < 5s p99 | 99.9% | 99.5% |
| Dashboard API | Response < 500ms p95 | 99.9% | 99.5% |
| Command execution | ACK < 10s p99 | 99% | 95% |
| Multi-tenant isolation | Zero cross-tenant data leaks | 100% | 100% |
| Data durability | Zero loss for ingested points | 99.999% (5 nines) | 99.99% |
| Platform uptime | API + dashboard reachable | 99.95% (≤4.4h/month down) | 99.5% (≤3.6h/month) |

### Non-Functional Targets

| Categorie | Target |
|---|---|
| **Throughput ingest** | 10,000 msg/sec sustained, 50,000 msg/sec burst (per Go ingest instance) |
| **Concurrent dashboard sessions** | 1,000 simultani per tenant cluster |
| **Tenant onboarding time** | < 5 min self-service, < 1h with admin assist |
| **Device onboarding time** | < 30 sec post-activation |
| **Cold start (process restart)** | < 30 sec to ready state |
| **Memory footprint** | Go ingest < 512 MB / 1000 devices; Django < 1 GB / worker |
| **Hard delete latency** (right-to-erasure) | < 24h to complete cross-storage |
| **Data residency** | Configurable per tenant (EU/US clusters) — Faza 10+ |

### Architecture Layers (Target State)

```
┌──────────────────────────────────────────────────────────────────┐
│ FRONTEND (React) — capability-driven UI                            │
│   Querry: "give me all devices with capability=inverter"           │
└─────────────────────────────┬────────────────────────────────────┘
                              │ REST + WebSocket (Faza 6+)
┌─────────────────────────────┼────────────────────────────────────┐
│ API GATEWAY (Kong)                                                 │
│   - JWT validation, rate limit, tenant header injection            │
└─────────────────────────────┬────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌───────────────┐   ┌──────────────────┐   ┌───────────────┐
│ DJANGO API    │   │ GO API (metrics) │   │ Go EVENT BUS  │
│ - CRUD        │   │ - Influx queries │   │ (Faza 8 SSE/  │
│ - Tenant mgmt │   │ - capabilities   │   │  WebSocket)   │
│ - Rules CRUD  │   │   per device     │   │               │
└───────┬───────┘   └────────┬─────────┘   └───────┬───────┘
        │                    │                      │
        ▼                    ▼                      ▼
┌──────────────────────────────────────────────────────────────────┐
│ RUNTIME LAYER (in-memory + Redis)                                  │
│   DeviceRuntime{ id, capabilities, last_seen, state, normalized }  │
└──────────────────────────────────┬───────────────────────────────┘
                                   │
            ┌──────────────────────┼──────────────────────┐
            ▼                      ▼                      ▼
   ┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐
   │ DEVICE REGISTRY  │   │ TOPIC MATCHER    │   │ CAPABILITY ENGINE│
   │ (YAML loader)    │   │ (regex+wildcard) │   │ (inheritance)    │
   └────────┬─────────┘   └────────┬─────────┘   └────────┬─────────┘
            │                      │                      │
            └──────────────┬───────┴──────────────────────┘
                           ▼
        ┌──────────────────────────────────────────────────┐
        │ INGEST PIPELINE (Go)                               │
        │ MQTT msg → matcher → parser → normalizer → Influx │
        │            ↓                              │       │
        │      runtime update                       │       │
        │            ↓                              │       │
        │      rule evaluator                       │       │
        └──────────────────────────────────────────┴───────┘
                           │
        ┌──────────────────┼──────────────────┐
        ▼                  ▼                  ▼
   ┌─────────┐       ┌──────────┐       ┌──────────┐
   │ INFLUX  │       │  REDIS   │       │  MySQL   │
   │ (TSDB)  │       │ (cache)  │       │ (metadata)│
   └─────────┘       └──────────┘       └──────────┘

         ↑ MQTT 5 (mTLS) ↑
   ┌──────────────────────────┐
   │ EMQX (broker)             │
   │  - HTTP auth via Django   │
   │  - shared subs            │
   │  - per-tenant ACL         │
   └──────────────────────────┘
                ↑
   ┌──────────────────────────┐
   │ DEVICES                   │
   │ (vendor-specific topics)  │
   └──────────────────────────┘
```

### YAML Device Definition Schema (proposed)

```yaml
# /configs/devices/huawei_sun2000_3phase.yaml
schema_version: "1.0"
id: huawei_sun2000_3phase
name: "Huawei SUN2000 3-phase Hybrid Inverter"
vendor: huawei
model: SUN2000

protocol: mqtt
identification:
  topic_match:
    - pattern: "/+/+/+/telemetry"
      extract: { device_id: "$1" }
    - pattern: "tenants/+/devices/+/up/telemetry"
      extract: { tenant_id: "$1", device_id: "$2" }

parser:
  type: json_with_measurements_array
  payload_path: "measurements"
  normalize:
    pv_input_power:    { source: "key=pv_input_power", unit: kW }
    grid_active_power: { source: "key=active_power", unit: kW }
    battery_soc:       { source: "key=battery_soc", unit: "%" }

capabilities:
  - inverter
  - battery
  - power_meter
  - solar_pv

commands:
  read_real_time:
    topic: "$prefix/{device_id}/down/cmd"
    payload: '{"action":"read_real_time"}'
    timeout_s: 10

telemetry_streams:
  telemetry: { interval_hint: 30s }
  shadow:    { interval_hint: 60s }
  cmd_ack:   { interval_hint: 0 }   # event-driven
```

---

## Phase Plan

> Pentru fiecare fază: **prerequisites**, **deliverables**, **exit criteria**, **risks**, **effort**, **owner**.

### Faza 1 — Audit & Architecture Documentation

**Status:** ✅ Completat prin acest document.

**Deliverables:**
- `docs/architecture.md` — diagrama target + ADRs
- `docs/migration_plan.md` — ordine fază + rollback per fază
- `docs/hardcoded_logic_report.md` — listă completă (în acest document §Current State)

**Exit criteria:**
- [x] Toate hardcoded patterns identificate
- [x] Target arhitectură aprobată de architecture board
- [x] Risk register revizuit
- [x] Sign-off prerequisites (vezi §Prerequisites Verification)

---

### Faza 2 — Device Definitions (YAML)

**Prerequisites:**
- ✅ Faza 1 audit complet
- ❌ JSON Schema validator chosen (recomandare: `cue` sau `gojsonschema`)
- ❌ YAML linter configurat (`yamllint` + pre-commit)

**Deliverables:**
- `configs/devices/*.yaml` — 4+ exemple (Huawei SUN2000, Nous A1T, Shelly EM, Zigbee2MQTT temp)
- `internal/registry/loader.go` — YAML loader + validator + cache
- `internal/registry/types.go` — `DeviceDefinition` struct
- Unit tests cu malformed YAML
- ADR `ADR-001-yaml-driven-devices.md`

**Exit criteria:**
- [ ] 4 YAML-uri valide loaded la startup
- [ ] `LoadDeviceDefinitions()` returnează count > 0
- [ ] Validare schema rejects malformed YAML cu eroare clară
- [ ] Test coverage > 80%

**Risks:**
- **R-2.1** YAML schema prea rigid → backward incompat la noi vendors. **Mitigare:** versioning `schema_version` + deprecation policy.
- **R-2.2** Hot-reload missing → restart Go la fiecare DD nou. **Mitigare:** SIGHUP reload în Faza 3.

**Effort:** 2–3 zile (16–24h).
**Owner:** Senior Go engineer + AI agent assist.

---

### Faza 3 — Topic Matcher Generic

**Prerequisites:**
- ✅ Faza 2 livrată
- ❌ MQTT topic ABNF understanding documentat (link la RFC + Tasmota/Shelly conventions)

**Deliverables:**
- `internal/matcher/matcher.go` — `MatchDeviceByTopic(topic) (*DeviceDefinition, extracted map)`
- Suport: `+` single-level wildcard, `#` multi-level, regex (`pattern:` în YAML), exact match
- Unit tests cu 30+ topic patterns reali
- Benchmark: matcher < 50µs p99 pentru 100 DD-uri
- Replace TOATE `strings.Contains` / `HasSuffix` din `cmd/main.go`

**Exit criteria:**
- [ ] Zero `strings.Contains(topic` și `strings.HasSuffix(topic` în business logic Go
- [ ] Bench `BenchmarkMatcher` < 50µs / op
- [ ] Backwards compat: testele existente pentru SUN2000/Tasmota încă pass
- [ ] Feature flag `MATCHER_ENABLED=true` documentat

**Risks:**
- **R-3.1** Match ambiguu (același topic prinde 2 DD) → comportament undefined. **Mitigare:** prioritate explicită + log warning + reject la load.
- **R-3.2** Performance regression sub load. **Mitigare:** prefix tree / trie pentru large DD count.

**Effort:** 3–5 zile (24–40h).
**Owner:** Senior Go engineer.

---

### Faza 4 — Parser Engine Generic

**Prerequisites:**
- ✅ Faza 3 livrată
- ❌ Decizie tipuri parser suportate (json, raw, keyvalue, modbus_tcp, csv?)

**Deliverables:**
- `internal/parsers/json.go`, `raw.go`, `keyvalue.go`, `measurements_array.go` (Huawei)
- `internal/parsers/parser.go` — `ParsePayload(dd, payload) (ParsedTelemetry, error)`
- Field normalization layer (vendor field → normalized name + unit conversion)
- Eliminare `StateMessage`, `SensorMessage`, `EnergyData` din `cmd/main.go`
- Tests: 50+ payload samples (din InfluxDB istoric)

**Exit criteria:**
- [ ] Zero `type StateMessage struct` etc. în business logic
- [ ] `ParsedTelemetry` flowă spre Influx writer
- [ ] Coverage > 85%
- [ ] Replay log: 24h MQTT capture rerun fără data loss

**Risks:**
- **R-4.1** Field name conflicts (vezi current `power` field type lock). **Mitigare:** namespace per vendor în `normalized_fields` + migration script `cleanup_field_locks.go`.
- **R-4.2** Unit conversion bugs (kW vs W, Wh vs kWh). **Mitigare:** unit metadata în normalized field + property-based testing.

**Effort:** 5–7 zile (40–56h).
**Owner:** Senior Go engineer + QA.

---

### Faza 5 — Capability Engine

**Prerequisites:**
- ✅ Faza 4 livrată
- ❌ Capability vocabulary aprobat (lista canonical: `inverter`, `battery`, `relay`, `power_meter`, `temperature_sensor`, `humidity_sensor`, `motion_sensor`, `light`, `thermostat`, etc.)

**Deliverables:**
- `internal/capabilities/registry.go` + inheritance support
- `GetDevicesByCapability(tenant_id, capability)` API
- Frontend: replace `device_type === 'sun2000'` cu `capabilities.includes('inverter')`
- Django `Device.capabilities` field (denormalizat din DD pentru query rapid)
- Migration `0011_device_capabilities.py`

**Exit criteria:**
- [ ] Sidebar/Layout filtrare pe capabilities (nu device_type)
- [ ] API `/devices/?capability=inverter` funcțional
- [ ] Capability inheritance test: `smart_plug` → moștenește `relay`+`power_meter`
- [ ] Documentație ADR `ADR-002-capability-vocabulary.md`

**Risks:**
- **R-5.1** Vocabulary explosion → 100+ capabilities → confuzie. **Mitigare:** review process + versioning + deprecation.
- **R-5.2** Backwards compat dashboard. **Mitigare:** feature flag UI + dual-source 4 săptămâni.

**Effort:** 4–6 zile (32–48h).
**Owner:** Full-stack engineer + UX review.

---

### Faza 6 — Device Runtime + State Engine

**Prerequisites:**
- ✅ Faza 5 livrată
- ❌ OpenTelemetry SDK adoptat (pentru tracing runtime updates)

**Deliverables:**
- `internal/runtime/manager.go` — `RuntimeManager` cu in-memory map + Redis sync
- `DeviceRuntime{ id, capabilities, last_seen, state, online, normalized_values }`
- Online/offline detection: `last_seen + heartbeat_threshold < now` → offline event
- API endpoint `GET /go/runtime/{device}` cu cache 1s
- Frontend live indicator pe device list

**Exit criteria:**
- [ ] Detecție offline < 60s post-disconnect (configurabil per DD)
- [ ] Memory < 1KB / device runtime
- [ ] Recovery la restart Go: rebuild runtime din ultimele 5 min Influx + Redis
- [ ] Integrare audit log: device offline → AuditLog entry

**Risks:**
- **R-6.1** Runtime drift între instanțe Go. **Mitigare:** Redis canonical + leader election dacă > 2 instanțe.
- **R-6.2** Memory leak pe long-running. **Mitigare:** TTL eviction + heap profile săptămânal.

**Effort:** 6–8 zile (48–64h).
**Owner:** Senior Go + SRE.

---

### Faza 7 — Command Engine Multi-Protocol

**Prerequisites:**
- ✅ Faza 6 livrată
- ✅ Faza 3.3 (commands existing) ca foundation
- ❌ Modbus client Go evaluat (recomandare: `simonvetter/modbus`)

**Deliverables:**
- Extindere YAML schema cu `commands:` block (vezi exemplu)
- `internal/commands/dispatcher.go` cu protocol routers (mqtt, modbus, http)
- Integrare cu `DeviceCommand` Django model existent
- Frontend: generic command builder UI per device capability
- Audit log per command (cine, când, pe ce device, payload, ack)

**Exit criteria:**
- [ ] Boiler ON/OFF prin YAML command (no hardcode în Django views)
- [ ] SUN2000 read register prin Modbus (proof of concept)
- [ ] Command timeout + retry exponențial
- [ ] Command audit log queryable cu `tenant_id` filter

**Risks:**
- **R-7.1** Modbus connection pooling complex. **Mitigare:** start cu MQTT only, Modbus în Faza 7.5.
- **R-7.2** Command replay attacks. **Mitigare:** nonce + timestamp + signing pentru cmd_ack.

**Effort:** 7–10 zile (56–80h).
**Owner:** Senior Go + Security review.

---

### Faza 8 — Rule Engine + Automations

**Prerequisites:**
- ✅ Faza 7 livrată
- ✅ Faza 4 (rules existing) ca foundation
- ❌ Rule DSL versioning policy aprobat

**Deliverables:**
- Extindere DSL existent cu: hysteresis, debounce, schedules (cron), state machines, timer-based
- `internal/rules/evaluator.go` cu event bus subscription pe runtime updates
- Dry-run mode: simulate rule pe istoric Influx fără side-effects
- Frontend rule builder cu templates per capability
- Rule audit log + execution history

**Exit criteria:**
- [ ] Rule "if solar_power > 5kW for 5 min then turn_on(boiler)" funcțională
- [ ] Hysteresis pe `temperature` (heating cycle test)
- [ ] Schedule cron syntax suportat
- [ ] Rule test framework (dry-run pe ultimele 7 zile)

**Risks:**
- **R-8.1** Rule explosion → load pe rule-engine. **Mitigare:** rate limit per tenant + circuit breaker.
- **R-8.2** Rule misfires (false positive). **Mitigare:** dry-run mandatory pentru noi rules + cooldown.

**Effort:** 8–12 zile (64–96h).
**Owner:** Senior Go + Product UX.

---

### Faza 9 — Enterprise Multi-Tenant Hardening

**Prerequisites:**
- ✅ Fazele 2–8 livrate
- ✅ Faza 1.9 hardening de bază (existing) ca foundation
- ❌ External penetration test scheduled

**Deliverables:**
- Tenant quotas (max devices, max msg/sec, max storage, max rules)
- Tenant-aware rate limiting în Kong + Go
- Cross-tenant audit log: zero leak verifiable
- RBAC granularity: capability-based permissions (ex: "can_send_command pe inverter")
- Data residency option (per tenant region pin)
- Tenant data export (GDPR portability) + delete (right-to-erasure)
- SOC 2 controls implementate
- Pen test report + remediations

**Exit criteria:**
- [ ] Pen test: zero critical/high findings
- [ ] GDPR right-to-erasure < 24h end-to-end (MySQL + Influx + Redis + backups)
- [ ] Tenant quota enforcement testat la limit
- [ ] Audit log retention 1y + tamper-evident (signed entries)

**Risks:**
- **R-9.1** Quota enforcement performance hit. **Mitigare:** Redis lua script pentru atomic check.
- **R-9.2** Compliance gap descoperit târziu. **Mitigare:** quarterly compliance review din Faza 5.

**Effort:** 10–15 zile (80–120h).
**Owner:** Security lead + SRE + Legal.

---

## Cross-Cutting Concerns

> Aplicabile **TOATE** fazelor.

### 1. Backwards Compatibility (Dual-Mode)

Pentru fiecare fază care schimbă ingest path:
- Feature flag `LEGACY_<COMPONENT>_ENABLED=true` permite paralel cu nou
- Dual-write 4 săptămâni (vechiul handler + noul YAML-driven)
- Compare reports săptămânale: percent match între pipeline-uri
- Switch over după 4 săpt cu zero divergence > 0.1%
- Remove legacy code 8 săptămâni post-switch

### 2. Migration Strategy

Pentru fiecare fază:
1. **Pre-deploy:** rulează scripts validation
2. **Deploy:** blue-green (2 instanțe), drain conn cu LB
3. **Post-deploy:** smoke test 15 min
4. **Rollback trigger:** dacă error rate > baseline + 2σ → automatic rollback

### 3. Feature Flags

Recomandare: **GrowthBook** sau **Unleash** self-hosted (open source).
Flag-uri necesare:
- `MATCHER_ENABLED`
- `PARSER_GENERIC_ENABLED`
- `RUNTIME_ENABLED`
- `RULE_DRY_RUN_ONLY`
- `TENANT_QUOTAS_ENFORCED`

### 4. Logging Standards

JSON structured pe tot Go, plus tags obligatorii: `tenant_id`, `device_id`, `trace_id`, `phase`.

Python (Django): `python-json-logger` cu request middleware care injectează `tenant_id`.

### 5. Tracing (OpenTelemetry)

Trace ID propagate de la MQTT → matcher → parser → runtime → Influx write. Permite debug end-to-end pe un mesaj.

### 6. Testing Pyramid

| Tip | Target % din effort | Target coverage |
|---|---|---|
| Unit | 60% | > 85% |
| Integration | 25% | Componenți + adjacencies |
| E2E | 10% | Critical user journeys |
| Chaos | 3% | Per quarter (network partitions, broker down) |
| Load | 2% | Pre-major release |

---

## Risk Register

| ID | Risk | Likelihood | Impact | Mitigation | Owner |
|---|---|---|---|---|---|
| R-1 | Breaking change ingest pipeline | Medium | Critical | Dual-mode 4 săpt + feature flags | Tech Lead |
| R-2 | YAML schema drift | Medium | High | `schema_version` + linter în CI | Architect |
| R-3 | Cross-tenant leak descoperit post-deploy | Low | Critical | Pen test + automated test suite | Security |
| R-4 | Performance regression > 20% | Medium | High | Bench gates în CI | SRE |
| R-5 | Rule engine misfire în prod | Medium | Medium | Dry-run mandatory | Product |
| R-6 | InfluxDB field type lock | High | Medium | Migration scripts + namespace | Senior Go |
| R-7 | Vendor SDK breaking change | Low | Medium | Pin versions + canary tests | Tech Lead |
| R-8 | Loss of devices în migration | Low | High | Backup pre-migration + replay log | SRE |
| R-9 | AI Agent introduce bugs subtile | High | Medium | Human review per phase + tests | Tech Lead |
| R-10 | Compliance gap (GDPR/SOC2) | Medium | Critical | Quarterly external audit | CISO |
| R-11 | Capacity exhausted (RAM/CPU) | Medium | High | Capacity planning + autoscale | SRE |
| R-12 | Vendor lock-in InfluxDB | Low | Medium | Abstraction layer + Postgres TimescaleDB pilot | Architect |

---

## Effort & Capacity Estimation

| Faza | Effort dev (h) | Effort review (h) | Calendar (săpt cu 1 FTE) |
|---|---|---|---|
| 1. Audit | 16 | 8 | 0.5 |
| 2. YAML DD | 24 | 8 | 1 |
| 3. Topic Matcher | 40 | 12 | 1.5 |
| 4. Parser Engine | 56 | 16 | 2 |
| 5. Capability | 48 | 16 | 1.5 |
| 6. Runtime | 64 | 16 | 2 |
| 7. Command Engine | 80 | 24 | 2.5 |
| 8. Rule Engine | 96 | 32 | 3 |
| 9. Tenant Hardening | 120 | 40 | 4 |
| **TOTAL** | **544** | **172** | **~18 săpt** |

**Plus prerequisite gaps** (CI/CD, observability, backup, compliance baseline): **+200–300h** (tooling + infra).

**Total realistic:** **~20–24 săptămâni cu 1 Senior FTE + AI agent assist.**

Cu 2 FTE paralel (split: backend + SRE/security) → **12–14 săpt.**

---

## Tooling Matrix

| Categorie | Tool | Uz | Faza intro |
|---|---|---|---|
| **Linting Go** | golangci-lint | Pre-commit + CI | Faza 2 |
| **Linting Python** | ruff + black + mypy | Pre-commit + CI | Faza 2 |
| **Linting YAML** | yamllint | Pre-commit | Faza 2 |
| **YAML Schema** | gojsonschema sau cue | Validation runtime | Faza 2 |
| **Test Go** | testify + go test | All phases | Always |
| **Test Python** | pytest + pytest-django + pytest-cov | All phases | Always |
| **Test E2E** | Playwright | Frontend critical paths | Faza 5 |
| **Bench Go** | go test -bench | Performance gates | Faza 3+ |
| **Load test** | k6 sau Locust | Pre-major release | Faza 6+ |
| **Security scan** | gosec, bandit, trivy | CI | Faza 1 |
| **Secret scan** | gitleaks | Pre-commit | Faza 1 |
| **Coverage** | codecov.io / coveralls | CI gate ≥ 80% | Faza 2 |
| **Tracing** | OpenTelemetry + Jaeger / Tempo | Runtime | Faza 6 |
| **Metrics** | Prometheus + Grafana | Runtime | Faza 1 |
| **Logs** | Loki sau ELK | Runtime | Faza 1 |
| **Errors** | Sentry | Runtime | Faza 5 |
| **Feature flags** | GrowthBook self-hosted | All phases | Faza 2 |
| **CI/CD** | GitHub Actions + ArgoCD (k8s) | Deploy | Faza 1 |
| **Secrets** | Vault sau AWS Secrets Manager | Runtime | Faza 9 |
| **Container reg** | GHCR | Build artifacts | Faza 9 |

---

## RACI & Approval Matrix

| Decizie / Faza | Architect | Tech Lead | Senior Dev | SRE | Security | Product | Legal |
|---|---|---|---|---|---|---|---|
| Architecture changes | A | R | C | C | C | I | I |
| YAML schema design | A | R | R | I | I | C | I |
| Phase implementation | I | A | R | I | I | I | I |
| Production deploy | C | A | R | R | C | I | I |
| Security audit | C | I | I | C | A | I | C |
| Compliance sign-off | C | I | I | C | C | I | A |
| Rule engine UX | I | I | C | I | I | A | I |
| Tenant quota policy | C | C | I | I | C | A | C |

R = Responsible, A = Accountable, C = Consulted, I = Informed.

---

## Quality Gates per Phase

Fiecare PR care implementează o fază trece prin:

```
┌─────────────────────────────────────────────────┐
│ PR Open                                          │
└────────────────┬────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────┐
│ Gate 1: Lint                                      │
│   - golangci-lint = 0 errors                      │
│   - ruff = 0 errors                               │
│   - yamllint = 0 errors                           │
│   - tsc --noEmit = 0 errors                       │
└────────────────┬────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────┐
│ Gate 2: Tests                                     │
│   - go test ./... = PASS                          │
│   - pytest = PASS                                 │
│   - npm test = PASS                               │
│   - Coverage >= 85% (per faza)                    │
└────────────────┬────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────┐
│ Gate 3: Security                                  │
│   - gosec = no HIGH                               │
│   - bandit = no HIGH                              │
│   - trivy scan = no CRITICAL                      │
│   - gitleaks = clean                              │
└────────────────┬────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────┐
│ Gate 4: Performance (Faza 3+)                     │
│   - Bench < baseline + 10%                        │
│   - Load test smoke (1K msg/s for 5 min)          │
└────────────────┬────────────────────────────────┘
                 ↓
┌─────────────────────────────────────────────────┐
│ Gate 5: Manual review                             │
│   - 2 approvers (1 Senior + 1 Security/SRE)       │
│   - Architecture board signoff (faze 2,5,9)       │
└────────────────┬────────────────────────────────┘
                 ↓
            Merge → Deploy via canary
```

---

## CI/CD Pipeline Requirements

```yaml
# .github/workflows/ci.yml — extended
name: CI Enterprise Gates

on: [push, pull_request]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: golangci/golangci-lint-action@v3
      - uses: chartboost/ruff-action@v1
      - run: yamllint configs/
      - run: cd dashboard && npx tsc --noEmit

  test-go:
    runs-on: ubuntu-latest
    services:
      redis: { image: redis:7 }
      influxdb: { image: influxdb:2.7 }
    steps:
      - uses: actions/checkout@v4
      - run: cd go-iot-platform && go test ./... -race -coverprofile=coverage.out
      - uses: codecov/codecov-action@v3

  test-python:
    runs-on: ubuntu-latest
    services:
      mysql: { image: mysql:8 }
    steps:
      - uses: actions/checkout@v4
      - run: cd django-bakend && pytest --cov=. --cov-fail-under=80

  test-frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: cd dashboard && npm ci && npm test && npm run build

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: securego/gosec@master
      - uses: pyupio/safety@2.3.5
      - uses: gitleaks/gitleaks-action@v2
      - uses: aquasecurity/trivy-action@master

  bench:
    if: github.event_name == 'pull_request' && contains(github.head_ref, 'faza-3')
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: cd go-iot-platform && go test -bench=. ./internal/matcher
```

**Branch protection rules pe `main`:**
- Necesită PR review (2 approvers)
- Necesită toate jobs CI green
- Necesită branch up-to-date with `main`
- Required status checks: lint, test-*, security
- No force push

---

## Disaster Recovery & Backup

### RPO / RTO Targets

| Date / Componentă | RPO | RTO |
|---|---|---|
| MySQL (tenant metadata, devices) | 1h (binlog) | 30 min |
| InfluxDB (telemetry) | 24h (snapshot) | 2h |
| Redis (cache only) | N/A (rebuildable) | 5 min (restart) |
| YAML configs | Real-time (Git) | Instant (re-deploy) |

### Backup Schedule

```
Daily 02:00 UTC:
  - MySQL full dump → S3 encrypted
  - InfluxDB snapshot per bucket → S3
  - Redis RDB → S3 (best effort)
  - Kong config export → S3

Hourly:
  - MySQL binlog → S3 (point-in-time recovery)

Weekly:
  - Full system snapshot (VM-level) → off-site
  - Restore drill (rotational: pick one component, restore to staging, verify)

Monthly:
  - Cross-region copy backup → off-region S3 bucket
  - Full DR exercise (kill primary, fail over to standby)

Quarterly:
  - Full RPO/RTO validation report
  - Update DR runbook
```

### DR Runbook (high-level)

1. **Incident detected** (alert / paging)
2. **Triage** — verifică Grafana, logs, last deploy
3. **Decision** — recover sau failover (criteria: > 30 min downtime → failover)
4. **Failover** — DNS switch / LB redirect → standby cluster
5. **Recovery** — restaurare backup pe primar, sincronizare delta
6. **Post-mortem** — < 5 zile, blameless, action items tracked

---

## Compliance & Security Checklist

### GDPR (mandatory pentru EU customers)

- [ ] **Lawful basis** documentat per data type
- [ ] **DPIA** completat pentru telemetry data sensibile
- [ ] **Data residency** EU (sau explicit consent transfer)
- [ ] **Right to access** — export per tenant cu device data
- [ ] **Right to erasure** — `delete_tenant.sh` cross-store + backup purge < 30 zile
- [ ] **Right to rectification** — admin UI permite edit
- [ ] **Data Processing Agreement** template
- [ ] **Sub-processors list** publicată
- [ ] **Breach notification** plan < 72h
- [ ] **Privacy by design** documentat
- [ ] **Cookie consent** dacă dashboard user-facing

### SOC 2 Type II (US enterprise)

- [ ] **CC1** Control environment (org chart, code of conduct)
- [ ] **CC2** Communication (incident reporting channels)
- [ ] **CC3** Risk assessment (annual review)
- [ ] **CC4** Monitoring (logs, alerts, IDS)
- [ ] **CC5** Control activities (access reviews, change mgmt)
- [ ] **CC6** Logical access (MFA, least privilege)
- [ ] **CC7** System operations (backup, recovery, IDS)
- [ ] **CC8** Change management (CI/CD, approvals)
- [ ] **CC9** Risk mitigation (security training, vuln mgmt)
- [ ] **A1** Availability (uptime monitoring, capacity)
- [ ] **C1** Confidentiality (encryption at rest + in transit)
- [ ] **PI1** Processing Integrity (data validation, monitoring)

### ISO 27001 (EU critical infra)

- Toate domeniile A.5–A.18 (114 controale).
- **Recomandare:** angajează consultant ISO + audit gap analysis.

### Security Hardening Checklist

- [ ] **TLS 1.3** everywhere (no plaintext interconnect)
- [ ] **mTLS** între services (cert per service)
- [ ] **JWT secret rotation** quarterly
- [ ] **bcrypt cost ≥ 12** (currently 12 default ✅)
- [ ] **Rate limit** per tenant + per IP
- [ ] **DDoS protection** la edge (Cloudflare / AWS Shield)
- [ ] **Dependency updates** weekly (Dependabot)
- [ ] **Vuln scan** daily (Trivy + Snyk)
- [ ] **Pen test** annual (extern)
- [ ] **Bug bounty** opt-in
- [ ] **Audit log retention** 1y minimum
- [ ] **Tamper-evident logs** (sign or hash chain)
- [ ] **Secret rotation policy** documentat
- [ ] **Incident response plan** testat

---

## Capacity Planning & Cost Model

### Scale Tiers

| Tier | Tenants | Devices | Msg/sec sustained | Total RAM | Total vCPU | Cost / lună (cloud) |
|---|---|---|---|---|---|---|
| **Pilot** | 1–10 | 10–100 | 100 | 16 GB | 8 | $200–400 |
| **SMB** | 10–100 | 100–1,000 | 1,000 | 64 GB | 24 | $800–1,500 |
| **Mid-market** | 100–1,000 | 1,000–10,000 | 10,000 | 256 GB | 64 | $3,000–6,000 |
| **Enterprise** | 1,000–5,000 | 10,000–50,000 | 50,000 | 1 TB | 200 | $15,000–30,000 |
| **Hyperscale** | 5,000+ | 50,000+ | 100,000+ | 4 TB+ | 500+ | Negotiated |

### Per-tenant cost estimate (mid-market)

| Componentă | Resursă | Cost / tenant / lună |
|---|---|---|
| MySQL row storage | ~10 MB / device | $0.01 |
| InfluxDB telemetry | ~1 GB / device / month | $0.30 |
| Redis cache | ~10 KB / device | $0.001 |
| Compute (shared) | 0.001 vCPU / device | $0.50 |
| Bandwidth (MQTT) | ~100 MB / device / month | $0.10 |
| **Total per device** | — | **$0.91 / device / month** |

Pentru un tenant cu 10 devices = ~$9/lună COGS. Tarif clientului recomandat: $29/lună (pricing 3x COGS pentru sustainable margins).

---

## Observability Stack

### The Three Pillars

```
┌──────────────────────────────────────────────────────────┐
│                                                            │
│  METRICS (Prometheus)        LOGS (Loki)        TRACES     │
│  - business KPIs             - structured        (Jaeger)  │
│  - SLI tracking              - tenant filtered   - cross   │
│  - alerting (Alertmanager)   - 30d retention       svc     │
│                                                            │
└──────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │  GRAFANA           │
                    │  - dashboards      │
                    │  - alerts UI       │
                    └──────────────────┘
```

### Required Dashboards

1. **Platform Overview** — uptime, msg/sec, error rate, latency p50/p95/p99
2. **Per-tenant** — devices online, msg ingested, rule executions, command success rate
3. **MQTT Pipeline** — broker connections, subscription health, parse errors
4. **InfluxDB Health** — write rate, query latency, disk usage per bucket
5. **Database Health** — MySQL connections, slow queries, replication lag
6. **Security** — failed logins, JWT errors, cross-tenant attempt count
7. **SLO Burn-rate** — alerting când burn-rate > 14.4x (1h window) sau 6x (6h)

### Required Alerts (PagerDuty / Opsgenie)

| Severity | Trigger | Action |
|---|---|---|
| **P1 critical** | Platform down > 2 min | Page on-call + escalation 5 min |
| **P1 critical** | Cross-tenant leak detected | Page Security + freeze writes |
| **P2 high** | SLO burn rate > 14.4x | Page on-call (Slack) |
| **P2 high** | DB connections > 80% | Page DB engineer |
| **P3 medium** | Error rate > baseline + 3σ | Slack alert |
| **P3 medium** | Single tenant quota exceeded | Notify customer + Slack |
| **P4 low** | Backup missing | Email Slack channel |

---

## AI Agent Development Conditions

> **Toate regulile din versiunea 1 sunt menținute** + adăugiri pentru enterprise.

### Reguli adiționale (v2)

#### 24. Phase prerequisites verification mandatory

AI Agent **NU începe** o fază înainte ca prerequisites pentru ea să fie ✅ în acest document.

#### 25. ADR mandatory pentru schimbări arhitecturale

Orice fază care introduce concept nou (matcher, parser, runtime, etc.) **necesită ADR** în `docs/adr/`. AI Agent generează draft, human aprobă.

#### 26. Performance regression check

La fiecare PR pe Faza 3+: rulează `go test -bench`. Dacă regression > 10%, **REJECT** PR și investighează.

#### 27. Security review per PR

PR-urile pe Faza 5+ (capability, command, rule, tenant) **necesită review de Security Engineer** (nu doar dev review).

#### 28. Migration script obligatoriu pentru schema changes

Orice migration MySQL sau Influx schema change **necesită**:
- Up + down migration
- Test pe staging cu producția shadow
- Rollback plan documentat

#### 29. Compliance impact analysis per phase

La planning fază: AI Agent identifică impacturi GDPR/SOC2 și le include în ADR.

#### 30. Capacity test pre-release

Înainte de major release (post-Faza 4, 7, 9): rulează **k6 load test** la 1.5x throughput target. Documentează rezultatele.

---

## Definition of Done — Enterprise

O fază e considerată **livrată** doar când TOATE check-urile sunt ✅:

### Code

- [ ] Codul compilează fără warnings
- [ ] `go test ./...` PASS, coverage ≥ 85% pe componenții noi
- [ ] `pytest` PASS, coverage ≥ 80%
- [ ] `npm test` PASS, type-check curat
- [ ] Niciun TODO / FIXME unhandled
- [ ] Niciun secret hardcoded (gitleaks clean)
- [ ] Linting clean (golangci-lint, ruff, yamllint)

### Architecture

- [ ] Niciun hardcoded vendor logic în business path
- [ ] Backwards compat verificat (dual-mode test)
- [ ] Feature flag funcțional
- [ ] Rollback procedure testată în staging

### Security

- [ ] Security scan PASS (gosec, bandit, trivy)
- [ ] Cross-tenant leak test PASS
- [ ] OWASP Top 10 review pentru noi endpoints
- [ ] Authentication & authorization verificate

### Performance

- [ ] Bench < baseline + 10%
- [ ] Load test PASS (1K msg/s sustained 5 min minim)
- [ ] Memory profile OK (no leaks)
- [ ] Latency p99 sub target

### Observability

- [ ] Metrics expuse (Prometheus)
- [ ] Logs structured cu tags obligatorii
- [ ] Tracing instrumentat (OpenTelemetry)
- [ ] Dashboard Grafana actualizat
- [ ] Alerts configurate

### Documentation

- [ ] `architecture.md` actualizat
- [ ] ADR creat (dacă schimbare arhitecturală)
- [ ] Runbook operațional pentru noua componentă
- [ ] API docs (drf-spectacular sau go-swagger) regenerate
- [ ] CHANGELOG.md updated
- [ ] README.md status table updated

### Process

- [ ] Commit format Conventional Commits
- [ ] PR review: 2 approvers (1 Senior + 1 Security/SRE)
- [ ] Architecture board sign-off (Faze 2, 5, 9)
- [ ] Compliance review (Faze 7, 9)
- [ ] Migration script testat (dacă aplicabil)
- [ ] Backup verified pre-deploy
- [ ] Smoke test post-deploy passed (15 min)
- [ ] Customer-facing changelog (dacă aplicabil)

---

## Documentation Standards

### Architecture Decision Records (ADR)

Format: `docs/adr/NNNN-title.md`

```markdown
# ADR-NNNN: <Title>

Date: YYYY-MM-DD
Status: Proposed | Accepted | Deprecated | Superseded by ADR-XXXX
Deciders: <names>

## Context
<Background, problem statement>

## Decision
<What we chose to do>

## Consequences
<Positive, negative, neutral>

## Alternatives Considered
<Options evaluated and why rejected>
```

### Runbooks (`docs/runbooks/`)

Pentru fiecare componentă majoră:
- Symptom & detection
- Triage steps
- Resolution steps
- Escalation
- Post-incident actions

### Capacity Reports (`docs/capacity/YYYY-Qx.md`)

Quarterly:
- Tenant growth trend
- Resource usage growth
- Forecast 6 luni
- Scaling recommendations

---

## Appendix A — Reference Implementations

### A.1 Existing Foundation (Faza 0–4 livrate)

| Component | Location | Maps to Upgrade Phase |
|---|---|---|
| Multi-tenant middleware | `django-bakend/tenants/middleware.py` | Faza 9 (extend) |
| Strict tenant Influx filter | `go-iot-platform/internal/influx/client.go` | Faza 9 (audit) |
| Device registration | `django-bakend/clients/views.py` DeviceViewSet | Faza 5 (extend cu capabilities) |
| Existing rule engine | `go-iot-platform/cmd/rule-engine/` | Faza 8 (extend DSL) |
| Existing command pipeline | `go-iot-platform/cmd/downlink-worker/` | Faza 7 (generalize) |
| Notifications | `django-bakend/notifications/` | Faza 8 (event integration) |
| OTA | `django-bakend/ota/` | Faza 7 (subset of commands) |

### A.2 External References

- MQTT 5.0 spec: https://docs.oasis-open.org/mqtt/mqtt/v5.0/
- InfluxDB Flux docs: https://docs.influxdata.com/flux/v0.x/
- OpenTelemetry semantic conventions: https://opentelemetry.io/docs/specs/semconv/
- SOC 2 Trust Services Criteria: AICPA TSP 100
- GDPR articles 17 (erasure), 20 (portability), 33 (breach notification)
- Tasmota commands: https://tasmota.github.io/docs/Commands/
- Huawei SUN2000 Modbus: vendor SDK + register map

### A.3 Recommended Reading

- "Site Reliability Engineering" (Google) — SRE practices
- "Building Microservices" (Sam Newman) — service design
- "Designing Data-Intensive Applications" (Martin Kleppmann) — data systems
- "Domain-Driven Design" (Evans) — bounded contexts
- "Release It!" (Michael Nygard) — production resilience patterns

---

## Final Notes

Acest roadmap este **living document**. La finalul fiecărei faze:

1. Update **§Current State Assessment** cu noul baseline
2. Update **§Risk Register** cu risks învățate
3. Update **§Effort estimation** cu actuals (pentru calibrare faze viitoare)
4. Add ADR-uri create
5. Tag în Git: `roadmap-vX.Y` la fiecare update major

**Owner final:** Architecture team. **Review trigger:** la finalul fiecărei faze + quarterly.

**Versiunea curentă:** 2.0 (Enterprise edition) — created 2026-05-10.

Versiuni viitoare:
- **v2.1** după Faza 2 livrată (concrete YAML schema decisions)
- **v3.0** după Faza 9 (post-enterprise readiness)
