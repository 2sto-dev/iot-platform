# iot-platform

Platformă IoT multi-tenant: Django (control plane), Go (data plane / MQTT ingest), EMQX (broker), InfluxDB (telemetrie), Kong (API gateway), MySQL (relațional).

## Status

| Faza | Status | Referință |
|------|--------|-----------|
| **Faza 0** — Stabilizare și fundație | ✅ Completă | [raport.md §1–6](raport.md) |
| **Faza 1** — Refactor multi-tenant | ✅ Completă, deployed | [raport.md §7–13](raport.md) |
| **Faza 1.9** — Hardening punctual | ✅ Completă | [raport.md §14–19](raport.md) |
| **Faza 2** — Ingest scalabil (EMQX, Redis, batch writes) | ✅ Completă (infra provisionată pe redis-1 + emqx-1) | [raport.md §20–24](raport.md) |
| **Faza 3** — Control plane device (credentials, activation, commands, shadow, OTA) | ✅ Completă, teste verzi | [plan.md §Faza 3](plan.md) |
| **Faza 4** — Rules engine + notifications + audit + api keys | ✅ Completă (billing rămâne pentru o fază viitoare) | [plan.md §Faza 4](plan.md) |
| **Faza 5** — Frontend dashboard React + UX Solar | ✅ Completă (RBAC, multi-tenant cache, Solar UX FusionSolar-style) | — |
| **Aggregare daily/monthly/yearly** | 📐 Design done, implementare deschisă | [agregare.md](agregare.md) |

### Faza 3 — Control plane device

| Sub-fază | Componentă | Status |
|---|---|---|
| 3.1 | Device credentials per-device (`mqtt_password_hash` BCrypt + endpoint `/credentials/rotate/`) | ✅ |
| 3.2 | Activation flow public (`/api/provisioning/activate/` + `manage.py generate_activation_token`) | ✅ |
| 3.3 | Downlink commands cu ACK tracking (`DeviceCommand` + Redis `cmd:queue` + Go `downlink-worker`) | ✅ |
| 3.4 | Device shadow (reported/desired/delta + push pe MQTT) | ✅ |
| 3.5 | OTA staged rollout (Firmware + RolloutPlan + DeviceOTAStatus + canary + auto-rollback) | ✅ |

### Faza 4 — Rules engine + platform features

| Componentă | Status |
|---|---|
| `rules/` Django app — DSL JSON, validator, signals Redis | ✅ |
| `notifications/` Django app — channels (webhook/email/FCM), sender async, events | ✅ |
| `audit/` Django app — log de evenimente + middleware | ✅ |
| `api_keys/` Django app — auth alternativ pt servicii externe | ✅ |
| Go `cmd/rule-engine` — evaluator DSL, cache Redis, executor acțiuni | ✅ |
| `internal/rules/` — condition tree, field path, render template | ✅ |
| Billing | ⏳ amânat (nu e prioritar pre-launch) |

## Componente

- **[django-bakend/](django-bakend/)** — Django REST API + 11 apps: `clients`, `tenants`, `provisioning`, `ota`, `audit`, `api_keys`, `rules`, `notifications` (+ migrații aplicate)
- **[go-iot-platform/](go-iot-platform/)** — Go services:
  - `cmd/main.go` — MQTT ingest scalabil cu validare device↔tenant + Influx batch writes
  - `cmd/mqtt-bridge/` — translator topics legacy (Shelly/Tasmota/Zigbee2MQTT) → schema tenant-scoped
  - `cmd/downlink-worker/` — consumer Redis `cmd:queue` → MQTT publish + ACK
  - `cmd/rule-engine/` — evaluator DSL + cache Redis + executor acțiuni
  - REST API metrici (`/go/metrics/{device}/{field}`)
- **[dashboard/](dashboard/)** — React 19 + Vite + Tailwind v4 + TanStack Query:
  - Pagini: Devices, Solar, Rules, Notifications, Audit Log
  - RBAC UI gating (`canWrite()` / `canSendCommands()`)
  - Multi-tenant React Query cache (queryKey `[..., tenant]`)
  - Solar UX: HouseLoadGauge, EnergyFlowDiagram animat (FusionSolar-style cu săgeți + SMIL), BatteryPanel + InverterPanel cu sortable metric cards
- **[kong/](kong/)** — API gateway: JWT validation + Lua `pre-function` plugin pt injectare `X-Tenant-Id`/`X-Role` la upstream Go
- **Externe:** EMQX (broker MQTT 5.0), InfluxDB (time-series, 3 buckets per plan), MySQL (auth + tenant data), Redis (cache device→tenant, queues comenzi/rules)

## Documentație

- [analiza.md](analiza.md) — evaluare stadiu actual vs. țintă (5k tenanți × 20k device-uri)
- [plan.md](plan.md) — roadmap fazat (0–4) cu dependențe explicite
- [raport.md](raport.md) — status detaliat + rezultate teste pe fiecare fază
- [agregare.md](agregare.md) — analiză + propunere pentru agregare daily/monthly/yearly în MySQL
- [api_schema.yaml](api_schema.yaml) — OpenAPI schema (drf-spectacular)

## Suite de teste

- Django: `pytest` în `django-bakend/` — **17 fișiere de test** (clients, tenants, provisioning, ota, rules, notifications, audit, api_keys)
- Go: `go test ./...` în `go-iot-platform/` — **10 pachete cu teste** (api, bridge, buffer, cache, influx, logging, ratelimit, rules, topics)
- CI: [.github/workflows/ci.yml](.github/workflows/ci.yml) — Django + Go paralel

## Deploy curent

### Backend
- 3+ tenants active (`legacy`, `integral`, `nedelcu`) în MySQL
- Device credentials per-device cu BCrypt + activation tokens (Faza 3.1+3.2)
- Service account `iot-ingest` + JWT-uri cu `tenant_id`, `tenant_slug`, `role`, `is_service`
- Kong propagă `X-Tenant-Id` / `X-Role` la upstream Go
- EMQX HTTP auth + ACL hooks Django → strict tenant isolation pe topice MQTT
- InfluxDB multi-bucket per plan (`iot-free` / `iot-pro` / `iot-enterprise`) cu strict `tenant_id` filtering la query

### Frontend
- React dashboard rulează prin Kong (`/api` → Django, `/go` → Go API direct)
- Login multi-tenant cu disambiguation (user în 2+ tenants)
- Solar page cu FusionSolar-style energy flow diagram + RBAC banners pt VIEWER

### MQTT topics
- Schema nouă: `tenants/{tid}/devices/{serial}/up/{stream}` cu validare strictă
- Legacy bridge pentru Shelly/Tasmota/Zigbee2MQTT (translatat la schema nouă)
- Streams suportate: `telemetry`, `shadow`, `cmd_ack`, `state`, `emeter`, `relay`
