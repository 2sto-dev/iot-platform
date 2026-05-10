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

### Topologie infrastructură

| VM | IP | Servicii | Porturi |
|---|---|---|---|
| **app-1** | `172.16.0.105` | Django (gunicorn), Go binaries (4×), MySQL client | `:8000` (Django), `:8090` (Go API) |
| **kong-1** | `172.16.0.106` | Kong API gateway (DB-less mode) | `:8000` (proxy), `:8001` (admin) |
| **redis-1** | provisionat | Redis 7 (cache + queues) | `:6379` |
| **emqx-1** | provisionat | EMQX 5 (broker MQTT) | `:1883` (TCP), `:8883` (TLS), `:8081` (management) |
| **influx-1** | provisionat | InfluxDB OSS 2.7 | `:8086` |

### Backend — Django (`172.16.0.105:8000`)

**Stack:** Django 5.2 + DRF 3.16 + djangorestframework-simplejwt 5.5 + drf-spectacular + MySQL (`mysqlclient` 2.2) + bcrypt 4.3 + paho-mqtt 2.1 + redis 5.2

**Apps active (10):**
- `clients` — Client (custom user), Device, DeviceShadow, DeviceCommand (10 migrații)
- `tenants` — Tenant (3 plan-uri × 3 status), Membership (5 roluri), middleware multi-tenant
- `provisioning` — ActivationToken cu hash SHA-256 + management command
- `ota` — Firmware, RolloutPlan, DeviceOTAStatus + `manage.py advance_rollout`
- `audit` — AuditLog + AuditMiddleware (capturează toate write requests)
- `api_keys` — APIKey cu scopes per tenant
- `rules` — Rule (DSL JSON), RuleExecution + signals Redis
- `notifications` — NotificationChannel, NotificationEvent + sender async daemon thread

**State:**
- 3 tenants active: `legacy` (`free`), `integral` (`pro`), `nedelcu` (`free`)
- Superuser `admin` cu acces cross-tenant (login cu `tenant_slug` → JWT `is_service=true`)
- Service account `iot-ingest` (perm `clients.view_device`) folosit de Go pentru Django API calls
- 6+ device-uri active: SUN2000 (`39371381`), Shelly EM, Zigbee Sensor (`412447`), Nous A1T

**Auth + multi-tenant:**
- JWT cu claims: `username`, `tenant_id`, `tenant_slug`, `role`, `is_service`, `iss="django"`, `exp`
- `TenantMiddleware` re-verifică Membership la fiecare request (cache TTL 60s în-process)
- `DeviceViewSet.get_queryset` preferă `request.tenant` peste cross-tenant fallback (anti leak superuser)
- Per-app permission classes: `IsAuthenticated + TenantRolePermission`
- Roluri suportate: `OWNER`, `ADMIN`, `OPERATOR`, `VIEWER`, `INSTALLER`

**MQTT integration (EMQX webhooks):**
- `POST /api/mqtt/auth/` — verificare credențiale device la CONNECT (bcrypt password check)
- `POST /api/mqtt/acl/` — autorizare PUBLISH/SUBSCRIBE per topic (validare `tenant_id` în topic match cu device-ul)

### Backend — Go services (`172.16.0.105:8090` API)

**Stack:** Go 1.21+, paho.mqtt.golang, influxdb-client-go v2, go-redis, golang-jwt v5

**4 binare independente (systemd services):**
- **`go-iot-platform`** (port 8090) — MQTT ingest + REST API metrici
  - Subscribe shared `$share/ingest/tenants/+/devices/+/up/#` (load balancing între workers)
  - Bridge legacy: `shellies/+`, `tele/+/+`, `zigbee2mqtt/+` → schema nouă
  - Validare device↔tenant la fiecare mesaj (cache Redis `device:{serial}` cu TTL 5min)
  - Rate limit per device + per tenant (token bucket în Redis)
  - Buffer fallback `/var/lib/iot/buffer.jsonl` când Influx pică (replay manual)
  - REST: `GET /metrics/{device}/{field}?range=15m` cu strict tenant filter pe Influx tag

- **`mqtt-bridge`** — translator dedicat pentru topics legacy (Shelly/Tasmota/Zigbee2MQTT)
  - Resubscribe pe legacy topics, republish pe schema `tenants/{tid}/devices/{serial}/up/{stream}`
  - Lookup device→tenant prin Redis cache (cu Django fallback)

- **`downlink-worker`** — consumer Redis BRPOP `cmd:queue`
  - Publish MQTT pe `tenants/{tid}/devices/{serial}/down/cmd` cu QoS 1
  - Update status `DeviceCommand.status=sent` prin Django service account
  - Timeout watch (5min) → status `failed` automat

- **`rule-engine`** — evaluator DSL pe streams MQTT
  - Cache Redis `rules:{tenant_id}` cu invalidare prin signals Django (Faza 4)
  - Subscribe `tenants/+/devices/+/up/+` shared
  - Evaluate condition tree pe field path + cooldown per rule
  - Execute actions: `notify` (POST către `/api/internal/notifications/trigger/`), `command` (push în `cmd:queue`)

### Backend — Kong gateway (`172.16.0.106:8000`)

**Mode:** DB-less, declarative config în `kong/kong.yaml` (în `.gitignore`, conține JWT secret)

**Routes configurate:**
- Public (fără JWT): `/api/token/`, `/api/token/refresh/`, `/api/auth/tenants/`, `/api/provisioning/`, `/api/schema/`, `/api/docs/`, `/api/redoc/`
- Protected JWT: `/api/devices/`, `/api/v1/rules/`, `/api/v1/notifications/`, `/api/v1/audit/`, `/api/v1/api-keys/`, `/api/ota/`
- Go upstream: `/go/*` cu plugin `pre-function` Lua care decodează JWT și injectează `X-Tenant-Id`, `X-Tenant-Slug`, `X-Role`, `X-Username` ca headers

**Plugin-uri active:** `jwt` (validation pe `iss=django`), `prometheus` (metrici), `pre-function` (Lua header injection)

**Notă:** ruta `/go` în Kong returnează 500 momentan (probabil `cjson.safe` indisponibil în plugin context); workaround = Vite proxy direct la Go pentru `/go/*` (Go re-validează JWT singur ca defense-in-depth).

### Backend — InfluxDB

**3 bucket-uri per plan tenant (retention diferit):**
- `iot-free` (retention 7d)
- `iot-pro` (retention 90d)
- `iot-enterprise` (retention 1y)

**Schema:**
- Measurement: `devices`
- Tags: `device` (serial), `tenant_id`, `source` (sun2000/shelly/zigbee2mqtt/nousat/generic), `type` (solar_inverter/power_meter/relay/sensor/etc.)
- Fields: payload-ul parsat (`pv_input_power`, `battery_soc`, `grid_power`, `house_load_kw_est`, `battery_temp`, etc.)

**Strict tenant isolation:** query Flux folosește `r.tenant_id == "{tid}"` exact (nu `or not exists` — fix anti-leak Faza 1.9 hardening).

**Bucket fallback la query:** `bucketsToTry(plan)` returnează ordinea `[primary, enterprise, pro, free, legacy]` deduplicat — pentru continuitate la upgrade plan (datele istorice rămân în bucket-ul vechi).

### Backend — Redis

**Usage:**
- `device:{serial}` (HASH) — cache device→tenant_id+plan, TTL 5min, populated lazy de Go ingest
- `cmd:queue` (LIST) — coadă comenzi downlink (LPUSH din Django, BRPOP din Go worker)
- `rules:{tenant_id}` (STRING JSON) — cache reguli per tenant, invalidat prin signals Django
- `ratelimit:{device}:*` (STRING) — token buckets per device + per tenant
- `notif:{event_id}:retry` (ZSET) — placeholder retry queue (planificat, neimplementat)

### Frontend — React Dashboard (`localhost:5173` dev, build static în prod)

**Stack:** React 19 + Vite 7 + TypeScript 5 + Tailwind CSS v4 + TanStack Query v5 + axios + react-router-dom v7 + @dnd-kit/core (sortable cards) + recharts (chart-uri)

**Build/dev:**
- Vite proxy: `/api` → Kong (`172.16.0.106:8000`), `/go` → Go direct (`172.16.0.105:8090`, ocolește Kong din cauza bug-ului Lua)
- Hot reload activ în dev pe port 5173/5174
- Build prod: `npm run build` → `dist/` → servit prin nginx/Kong static

**Auth flow:**
- `LoginPage` cu 2 pași: credentials → tenant disambiguation (dacă userul are membership în 2+ tenants)
- JWT salvat în `localStorage` (`access_token`, `refresh_token`, `tenant_slug`, `role`)
- Auto-refresh la 401 prin axios interceptor; logout pe refresh fail
- `lib/auth.ts` expune `canWrite()`, `canSendCommands()`, `getRole()`

**Multi-tenant cache discipline:**
- Toate query-urile folosesc `queryKey: ["devices", tenant]` etc. — cache izolat per tenant
- La switch tenant, cache-ul se invalidează automat (queryKey schimbat)
- Sidebar `Layout.tsx` filtrează dinamic link-urile pe baza device-urilor tenantului (ex: Solar apare doar dacă există minim un `sun2000`)

**Pagini active (5):**

| Pagină | Funcționalități |
|---|---|
| **Devices** | Listă tabelă + Add/Edit/Delete (modal) cu validare per device_type. RBAC: VIEWER read-only. |
| **Solar** | FusionSolar-style live overview (EnergyFlowDiagram cu săgeți + dashes animate SMIL), Bilanțul Energetic (Solar/Grid/Battery/House cu icoane), TemperatureCard inverter+battery cu threshold colorat, secțiuni Production (InverterPanel) + Storage (BatteryPanel) cu sortable metric grid drag-to-reorder (persistat localStorage) |
| **Rules** | Listă reguli + RuleBuilder cu drag-and-drop pentru condition tree (AND/OR/NOT), action list, cooldown, stream pattern. Toggle enable/disable + Delete cu confirm. |
| **Notifications** | Channels CRUD (webhook/email/FCM cu config dinamic per type), Send Test button (creează NotificationEvent), event history cu auto-refresh la 15s. RBAC banner amber pt VIEWER. |
| **Audit Log** | Tabel evenimente cu actor + action + resource + IP + timestamp, refresh la 30s. Read-only natural. |

**RBAC UI gating (defense in depth):**
- `canWrite()` (OWNER + ADMIN) ascunde butoane Add/Edit/Delete pentru VIEWER/OPERATOR
- `canSendCommands()` (OWNER + ADMIN + OPERATOR) — placeholder pentru viitoare comenzi UI
- Backend rămâne gardianul real (`TenantRolePermission` + `_require_write`); UI gating e doar UX

**Solar page UX detaliat:**
- **EnergyFlowDiagram** — SVG diamond layout (PV top, Battery left, Inverter center hub, Grid right, House bottom), linii cu `<animate>` pe `stroke-dashoffset` pentru flux animat, săgeți `<marker>` care se schimbă cu direcția (export/import, charge/discharge), legendă cu badge-uri active/inactive
- **Bilanțul Energetic** card lateral cu icon bubbles colorate per tip flux (amber pt PV, indigo pt charge, emerald pt discharge/export, rose pt import/load)
- **TemperatureCard** cu progress bar pe scala 0-80°C și status (Normal/Warm/Hot) per device
- **Section headers** cu accent colorat (vertical bar + eyebrow + title + subtitle + bg tonal + border) în 4 tonuri semantice
- **Sortable metric grid** — fiecare card draggable, ordinea persistă în `localStorage` cu key per tenant + group

### MQTT topics & streams

**Schema nouă (preferred):**
- Up: `tenants/{tid}/devices/{serial}/up/{stream}` (publicat de device sau bridge)
- Down: `tenants/{tid}/devices/{serial}/down/{kind}` (publicat de Go downlink-worker)
- Streams up suportate: `telemetry`, `shadow`, `cmd_ack`, `state`, `emeter`, `relay`, `tele`, `zigbee`
- Kinds down: `cmd`, `shadow_delta`

**Legacy bridges (translatori automați):**
- `shellies/{serial}/{stream}` → `tenants/{tid}/devices/{serial}/up/{stream}` (Shelly EM, Plus)
- `tele/{serial}/{stream}` → `tenants/{tid}/devices/{serial}/up/{stream}` (Tasmota / NousAT)
- `zigbee2mqtt/{serial}` → `tenants/{tid}/devices/{serial}/up/zigbee` (Z2M gateway)
- `/{serial}/{collector}/{device_sn}/telemetry` → `tenants/{tid}/devices/{serial}/up/telemetry` (Huawei SUN2000)

**Validări la ingest:**
- Topic format strict (drop tăcut dacă nu match regex)
- `device` în topic trebuie să existe în Django `Device` table
- `tenant_id` din topic trebuie să match cu `device.tenant_id` (drop "device-tenant mismatch")
- Rate limit per device + per tenant (token bucket Redis)
- Buffer fallback la `/var/lib/iot/buffer.jsonl` dacă Influx down (caller monitorizează size, replay manual)

### Observability

- Structured logging JSON pe Go (level + fields ca `device_id`, `tenant_id`, `topic`, `error`)
- Prometheus metrici prin Kong plugin (`prometheus`)
- Audit log Django capturează toate write requests cu actor + IP + payload diff
