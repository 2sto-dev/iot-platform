# iot-platform

Platformă IoT multi-tenant: Django (control plane), Go (data plane / MQTT ingest), EMQX (broker), InfluxDB (telemetrie), Kong (API gateway), MySQL (relațional).

## Status

| Faza | Status | Referință |
|------|--------|-----------|
| **Faza 0** — Stabilizare și fundație | ✅ Completă | [raport.md §1–6](raport.md) |
| **Faza 1** — Refactor multi-tenant | ✅ Completă, deployed | [raport.md §7–13](raport.md) |
| **Faza 1.9** — Hardening punctual | ✅ Completă | [raport.md §14–19](raport.md) |
| **Faza 2** — Ingest scalabil (EMQX cluster, Redis, batch writes) | ⏳ Următoarea | [plan.md §Faza 2](plan.md) |
| **Faza 3** — Control plane device (provisioning, downlink, OTA) | ⏳ | [plan.md §Faza 3](plan.md) |
| **Faza 4** — Funcționalități platformă (rules, billing, audit) | ⏳ | [plan.md §Faza 4](plan.md) |

## Componente

- **[django-bakend/](django-bakend/)** — Django REST API: auth, Tenant + Membership, Device CRUD, RBAC tenant-aware
- **[go-iot-platform/](go-iot-platform/)** — Go service: MQTT ingest cu validare device↔tenant, REST API metrici, structured logging
- **[kong/](kong/)** — Kong gateway cu JWT validation + propagare X-Tenant-Id la upstream
- Externe: EMQX (broker), InfluxDB (time-series), MySQL (auth + tenant data)

## Documentație

- [analiza.md](analiza.md) — evaluare stadiu actual vs. țintă (5k tenanți × 20k device-uri)
- [plan.md](plan.md) — roadmap fazat (0–4) cu dependențe explicite
- [raport.md](raport.md) — status detaliat + rezultate teste pe fiecare fază

## Suite de teste

- Django: `pytest` în `django-bakend/` — 52 teste
- Go: `go test ./...` în `go-iot-platform/` — 6 pachete cu teste
- CI: [.github/workflows/ci.yml](.github/workflows/ci.yml) — Django + Go paralel

## Deploy curent (post-Faza 1)

- 1 tenant `legacy` în MySQL (creat la migrarea 0005)
- Toate device-urile existente atribuite legacy tenant
- Service account `iot-ingest` provisionat
- JWT-uri conțin `tenant_id`, `tenant_slug`, `role`
- Kong propagă `X-Tenant-Id` la upstream
- Telemetria Influx tag-ată cu `tenant_id`
