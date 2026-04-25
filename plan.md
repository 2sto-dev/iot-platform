# Plan refactorizare platformă IoT — pași corelați

## Context

Sursă: [analiza.md](analiza.md) (2026-04-25). Verdictul analizei este că platforma actuală e un MVP single-tenant care reprezintă ~5–10% dintr-o platformă tip Tuya și **nu poate scala** la ținta de 5.000 tenanți × 20.000 device-uri fără schimbări fundamentale.

Acest plan transformă recomandările din §6 ale analizei într-un șir de **pași concreți, corelați** (cu precondiții explicite) astfel încât refactorizarea să se poată face **incremental** fără big-bang. Fiecare pas indică fișierele atinse și criteriile „done when".

**Principii care guvernează tot planul:**
1. **Niciun pas din Faza 2+ nu începe înainte ca Faza 1 să fie completă** — orice optimizare de scaling făcută peste single-tenant devine datorie tehnică dublă.
2. Fiecare pas trebuie să rămână **deployable** — nu se rup migrații sau API-uri fără shim.
3. **Tenant-id în JWT** este sursa unică de adevăr pentru izolare; toate componentele (Django, Go, Kong, MQTT broker) trebuie să-l consume.
4. Scrierea de teste și a unei pipeline CI minime (Faza 0) este precondiție pentru orice modificare din Faza 1+, ca refactor-ul să poată fi validat repetabil.

---

## Faza 0 — Stabilizare și fundație de lucru (urgent, nu pune layer-e peste cod nesigur)

Scop: securitate de bază + infrastructură reproductibilă + suite de testare. Toate sub-sarcinile pot fi făcute **în paralel**, dar toate trebuie încheiate înainte de Faza 1.

### 0.1 Secret hygiene
- **Depinde de:** —
- **Fișiere:** [kong/kong.yaml](kong/kong.yaml#L16), [django-bakend/django_backend/settings.py](django-bakend/django_backend/settings.py), [.env](go-iot-platform/.env)
- **Acțiune:**
  - Mută `secret: "123456789"` din kong.yaml într-un env var (`KONG_JWT_SECRET`) injectat la deploy.
  - Sincronizează cu Django `SIMPLE_JWT.SIGNING_KEY` (ambele citesc același secret).
  - Adaugă `.env` la `.gitignore` (verifică), `.env.example` cu chei goale.
- **Done when:** kong.yaml nu mai conține valori secrete; `grep -r 123456789` în repo returnează 0 hit-uri.

### 0.2 Service account dedicat pentru Go (eliminare superuser)
- **Depinde de:** —
- **Fișiere:** [go-iot-platform/cmd/main.go:50](go-iot-platform/cmd/main.go#L50), [django-bakend/clients/](django-bakend/clients/)
- **Acțiune:**
  - Creează în Django un user de serviciu `iot-ingest` cu permisiuni minime (citire devices, scriere devices auto-discovered).
  - Înlocuiește `DJANGO_SUPERUSER`/`DJANGO_SUPERPASS` în Go cu `DJANGO_SERVICE_USER`/`DJANGO_SERVICE_PASS`.
- **Done when:** Go-ul pornește autentificat ca user non-superuser și apelurile sale către Django funcționează.

### 0.3 Refuz auto-register în loc de `ClientID: 1` hardcodat
- **Depinde de:** —
- **Fișiere:** [go-iot-platform/cmd/main.go:163-193](go-iot-platform/cmd/main.go#L163-L193)
- **Acțiune:** scoate hardcodarea `ClientID: 1`. Până când Faza 1 introduce tenant-mapping, log-uiește mesajul și **refuză** scrierea în Django pentru device-uri necunoscute (continuă să scrie în Influx cu tag `unassigned=true`, ca să nu pierzi telemetria, dar nu mai poluezi tabela `Device` cu atribuiri greșite).
- **Done when:** un device nou nu mai apare automat la userul cu id=1.

### 0.4 CORS strict
- **Depinde de:** —
- **Fișiere:** [go-iot-platform/internal/api/middleware.go:10](go-iot-platform/internal/api/middleware.go#L10)
- **Acțiune:** înlocuiește `*` cu listă explicită citită din env (`ALLOWED_ORIGINS`). Default safe (lista goală → reject).
- **Done when:** request din origin neautorizat primește 403; din origin autorizat trece.

### 0.5 CI minim + scheletul de teste
- **Depinde de:** —
- **Fișiere noi:** `.github/workflows/ci.yml`, [django-bakend/clients/tests/](django-bakend/clients/tests/) (înlocuiește [clients/tests.py](django-bakend/clients/tests.py)), `*_test.go` în pachetele Go, `django-bakend/requirements-dev.txt`.
- **Acțiune:**
  - Django: pytest + pytest-django + factory_boy, primul test pe `DeviceViewSet` (SQLite în-memory pentru CI ca să eviți dependența de MySQL).
  - Go: `go test ./...`, primul test pe parsarea topic-urilor în `handleMessage` și pe `getUsernameFromToken`.
  - CI: jobs paralele cu `actions/setup-python` + `actions/setup-go` direct: `lint` (ruff + golangci-lint), `django-test`, `go-test`, `go-build`.
- **Done when:** workflow-ul rulează verde pe main; testele se rulează și local cu `pytest` / `go test ./...`.

### 0.6 Cleanup tehnic punctual
- **Depinde de:** —
- **Fișiere:** [go-iot-platform/cmd/main.go](go-iot-platform/cmd/main.go), [go-iot-platform/internal/influx/client.go:26](go-iot-platform/internal/influx/client.go#L26), [django-bakend/requirements.txt](django-bakend/requirements.txt), [django-bakend/django_backend/settings.py:83-85](django-bakend/django_backend/settings.py#L83-L85), [.gitignore](.gitignore)
- **Acțiune:**
  - Elimină `//go:embed go_meeter.log` (fișier static în binar fără rost).
  - Înlocuiește `strings.Title` cu `golang.org/x/text/cases`.
  - Adaugă graceful shutdown (signal handler peste `select {}` + `http.Server.Shutdown` cu timeout).
  - Parametrizează `range: -5m` din Flux query (default + override din query string `?range=15m`, validat regex).
  - Repară encoding `requirements.txt` (UTF-8 fără BOM).
  - Elimină `REST_FRAMEWORK` duplicat din settings.py (primul dict e dead code, override-ed de al doilea).
  - Repară `.gitignore` (`*.mod` exclude eronat toate `go.mod` — păstrează doar `cmd/go.mod` ca pattern explicit dacă e cazul, sau scoate complet după ce confirmăm că nu mai apar `go.mod` imbricate).
- **Done when:** `go vet` și `go build` curate; query-ul Influx acceptă `?range=15m`; settings.py are un singur `REST_FRAMEWORK`.

---

## Faza 1 — Refactor multi-tenant (fundația întregii platforme)

Scop: introducerea conceptului de **Tenant** la toate nivelele (DB, JWT, API, log-uri). Toți pașii din această fază sunt **strict secvențiali** — fiecare îl pregătește pe următorul.

### 1.1 Modelare Django: Tenant + Membership + Role
- **Depinde de:** Faza 0 completă
- **Fișiere noi:** `django-bakend/tenants/` (app nouă) cu `models.py`, `admin.py`, `serializers.py`, `views.py`, `urls.py`.
- **Acțiune:**
  - `Tenant(name, slug, plan, status, created_at)`.
  - `Membership(user FK, tenant FK, role enum {OWNER, ADMIN, OPERATOR, VIEWER, INSTALLER}, created_at)` cu unique `(user, tenant)`.
  - Migrare 0001 a app-ului `tenants`.
  - Înregistrare app în [settings.py INSTALLED_APPS](django-bakend/django_backend/settings.py).
- **Done when:** se pot crea tenanți și membership-uri din admin; testele pentru CRUD trec.

### 1.2 Adăugare `tenant_id` la `Device`
- **Depinde de:** 1.1
- **Fișiere:** [django-bakend/clients/models.py:13-27](django-bakend/clients/models.py#L13-L27)
- **Acțiune:**
  - `tenant = ForeignKey(Tenant, on_delete=PROTECT, related_name="devices")` (nullable inițial pentru migrare safe).
  - Nou index: `Index(fields=["tenant", "serial_number"])`.
  - Schimbă constraint: `unique_together = ("tenant", "serial_number")` (scoate `unique=True` global).
  - Migrare în 2 pași: (a) adaugă coloana nullable, (b) populează (creează un tenant „legacy" care preia toate device-urile existente), (c) face NOT NULL + unique compus.
- **Done when:** toate device-urile existente au `tenant_id` populat; constraint nou activ.

### 1.3 JWT include `tenant_id` și `roles`
- **Depinde de:** 1.1
- **Fișiere:** [django-bakend/clients/tokens.py:1-12](django-bakend/clients/tokens.py#L1-L12), [django-bakend/clients/views.py](django-bakend/clients/views.py) (login flow)
- **Acțiune:**
  - La login, dacă userul are membership pe ≥2 tenanți, login-ul cere selecția tenantului (param `tenant_slug`); dacă are 1, e implicit.
  - Token include `tenant_id`, `tenant_slug`, `roles` (lista din membership).
  - Refresh token păstrează același tenant context.
- **Done when:** un token decodat conține câmpurile noi; testele care decodează JWT trec.

### 1.4 Middleware Django + manager queryset tenant-aware
- **Depinde de:** 1.2, 1.3
- **Fișiere noi:** `django-bakend/tenants/middleware.py`, `django-bakend/tenants/managers.py`. Modificări în [django-bakend/clients/views.py](django-bakend/clients/views.py).
- **Acțiune:**
  - `TenantMiddleware` extrage `tenant_id` din `request.auth` (JWT claims) și-l pune în `request.tenant`.
  - `TenantManager` cu `get_queryset().filter(tenant=current_tenant())` — folosit pentru `Device.objects`.
  - Toate viewsets trec prin manager-ul tenant-aware (sau filtrare explicită dacă `current_tenant()` e None).
  - Endpointul vechi `GET /api/devices/<username>/` devine `GET /api/devices/?username=...` cu filtrare implicită pe tenant.
- **Done when:** un user din tenantul A nu poate vedea device-urile tenantului B nici prin API, nici prin admin.

### 1.5 RBAC explicit
- **Depinde de:** 1.4
- **Fișiere:** `django-bakend/tenants/permissions.py`, modificări `permission_classes` în viewsets.
- **Acțiune:** `IsTenantAdmin`, `IsTenantOperator`, `IsTenantViewer`, etc. Mapare endpoint → roluri permise.
- **Done when:** un VIEWER nu poate face POST/DELETE pe `/api/devices/`.

### 1.6 Kong: consumer per tenant și `tenant_id` în log/headers
- **Depinde de:** 1.3
- **Fișiere:** [kong/kong.yaml](kong/kong.yaml)
- **Acțiune:**
  - Plugin `jwt2header` (sau `request-transformer` cu template) care propagă `X-Tenant-Id` din claim către upstream.
  - File-log îmbogățit cu `tenant_id` per request.
  - (Opțional) plugin `rate-limiting` per consumer/tenant.
- **Done when:** request-ul ajunge la upstream cu header-ul `X-Tenant-Id` setat corect.

### 1.7 Refactor Go API pentru tenant-awareness
- **Depinde de:** 1.6
- **Fișiere:** [go-iot-platform/internal/api/handlers.go:50-112](go-iot-platform/internal/api/handlers.go#L50-L112), [go-iot-platform/internal/django/client.go](go-iot-platform/internal/django/client.go), [go-iot-platform/internal/influx/client.go:24-29](go-iot-platform/internal/influx/client.go#L24-L29)
- **Acțiune:**
  - Extrage `tenant_id` din JWT (după `username`).
  - Cererea către Django pentru autorizare include header `X-Tenant-Id` și folosește endpoint nou `/api/devices/?username=&tenant=`.
  - Flux query Influx adaugă filtru `r.tenant_id == "<tid>"`.
- **Done when:** request-uri cu token de tenant A pe device de tenant B → 403.

### 1.8 Toate scrierile MQTT → Influx etichetate cu `tenant_id`
- **Depinde de:** 1.2, 1.7
- **Fișiere:** [go-iot-platform/cmd/main.go:151-297](go-iot-platform/cmd/main.go#L151-L297)
- **Acțiune:**
  - Toate `influxdb2.NewPoint("devices", tags, ...)` adaugă tag `tenant_id` (lookup din cache device→tenant, vezi 2.4 pentru cache; pe interim, lookup HTTP la fiecare scriere — slow dar corect).
  - Telemetria de la device-uri „necunoscute" merge cu tag `tenant_id="unassigned"`.
- **Done when:** orice punct nou în Influx are tag `tenant_id`; query-urile Faza 1.7 funcționează.

---

## Faza 2 — Ingestie scalabilă (data plane)

Precondiție absolută: **Faza 1 încheiată.** Fără tenant_id în pipeline, scaling-ul reproduce defecte la scară mai mare.

### 2.1 MQTT broker clusterizabil cu ACL
- **Depinde de:** 1.6 (consumer pattern), 1.8 (tenant tagging)
- **Fișiere:** instalare nativă EMQX/VerneMQ pe gazdă (binar oficial sau systemd unit), schemă topic-uri.
- **Acțiune:**
  - Înlocuiește Mosquitto cu **EMQX** (open source, are ACL via HTTP hook).
  - Topic schema nouă: `tenants/{tid}/devices/{did}/up/{stream}` și `down/cmd`.
  - ACL via webhook către Django: la publish/subscribe, EMQX întreabă Django dacă device-ul are dreptul la topic.
- **Done when:** un device cu credențiale tenant A nu poate publica pe topic tenant B.

### 2.2 MQTT bridge pentru device-uri legacy (Shelly/NousAT/Zigbee2MQTT)
- **Depinde de:** 2.1
- **Fișiere noi:** `go-iot-platform/internal/bridge/`
- **Acțiune:** translator care primește pe topic-uri vendor (ex. `shellies/{serial}/...`) și republică pe `tenants/{tid}/devices/{did}/up/...` după lookup serial→tenant.
- **Done when:** un mesaj Shelly ajunge la consumeri pe topic-ul tenant-aware.

### 2.3 Go-ul devine worker stateless cu shared subscription
- **Depinde de:** 2.1
- **Fișiere:** [go-iot-platform/cmd/main.go:97-115](go-iot-platform/cmd/main.go#L97-L115)
- **Acțiune:**
  - Subscribe la `$share/ingest/tenants/+/devices/+/up/#` (load-balanced între instanțe).
  - Elimină `Subscribe("#")`.
  - Permite `N` instanțe în paralel (deploy ca Deployment cu `replicas: N`).
- **Done when:** trei instanțe simultane consumă mesajele fără duplicare.

### 2.4 Cache device→tenant în Redis cu invalidare
- **Depinde de:** 2.3, 1.4
- **Fișiere:** `go-iot-platform/internal/cache/`, signals în Django (`django-bakend/clients/signals.py`).
- **Acțiune:**
  - Redis store: `device:{did} → {tenant_id, allowed_topics}`.
  - Django publică invalidări pe Redis pub/sub la `Device.save()`/`delete()`.
  - Go citește din cache; miss → fallback HTTP la Django + populare cache.
- **Done when:** schimbare tenant pe un device se propagă la Go în <1s; rata de hit cache > 99%.

### 2.5 Influx batch writes
- **Depinde de:** 2.3
- **Fișiere:** [go-iot-platform/cmd/main.go:48,57](go-iot-platform/cmd/main.go#L48)
- **Acțiune:** înlocuiește `WriteAPIBlocking` cu `WriteAPI` (async, batch). Configurare `BatchSize=5000`, `FlushInterval=1s`. Handler pentru erori pe `Errors()` channel.
- **Done when:** debit susținut de 5k+ msg/s la o singură instanță Go pe un laptop dev.

### 2.6 (Opțional) Buffer Kafka/NATS între MQTT și Influx
- **Depinde de:** 2.5
- **Fișiere:** infra + producer/consumer Go.
- **Acțiune:** dacă vârfurile depășesc capacitatea Influx, buffer-ul absoarbe; permite reprocesare la incidente Influx.
- **Done when:** kill -9 pe Influx writer nu pierde mesaje (rămân în topic).

### 2.7 Multi-bucket Influx pe tenant (sau retention policy per plan)
- **Depinde de:** 1.8, 2.5
- **Fișiere:** Go writer + provisioning script.
- **Acțiune:**
  - La crearea unui tenant (1.1), creează automat un bucket dedicat (sau folosește un bucket per tier de plan).
  - Writer-ul rutează pe bucket bazat pe `tenant_id`.
  - Retention diferit pe tier: free=7d, paid=90d, enterprise=2y.
- **Done when:** Două tenant-uri cu plan diferit au retention diferit verificabil.

---

## Faza 3 — Control plane device (downlink + provisioning)

Precondiție: Faza 2 încheiată (avem ACL, topology corectă, cache).

### 3.1 Provisioning service: device credentials
- **Depinde de:** 2.1
- **Fișiere noi:** app Django `provisioning/` cu `models.py` (DeviceCredential cu hash), `views.py`, endpoint `/api/devices/{id}/credentials/rotate/`.
- **Acțiune:** la crearea unui device se generează `device_secret` (hash bcrypt în DB, plain returnat o singură dată). EMQX folosește acest credential pentru auth via webhook (2.1).
- **Done when:** un device se autentifică la broker cu credențialele lui și e refuzat dacă sunt revocate.

### 3.2 Activation flow
- **Depinde de:** 3.1
- **Fișiere:** `provisioning/activation.py`, endpoint public `POST /api/activate/` cu activation_token.
- **Acțiune:** device-ul e fabricat cu un activation_token de o singură utilizare; la prima conectare îl schimbă pe `device_secret` permanent + asociere la tenant.
- **Done when:** un activation_token nu poate fi reutilizat; device-ul devine asociat tenantului corect.

### 3.3 Downlink command path
- **Depinde de:** 2.1, 3.1
- **Fișiere noi:** Go service nou `cmd/cmd-publisher/`, endpoint Django `POST /api/devices/{id}/commands/`.
- **Acțiune:** API → enqueue (Kafka/Redis) → publisher MQTT pe `tenants/{tid}/devices/{did}/down/cmd` → device răspunde pe `up/cmd_ack` → status în DB.
- **Done when:** API request → comandă executată → ACK propagat înapoi în <1s în condiții normale.

### 3.4 Device shadow (last reported / desired state)
- **Depinde de:** 3.3
- **Fișiere noi:** model `DeviceShadow` (JSONField), endpoint `GET/PATCH /api/devices/{id}/shadow/`.
- **Acțiune:** stochează ultima stare raportată + dorită (delta replay). La conectare device, broker trimite delta.
- **Done when:** modificarea desired state declanșează comandă automat când device-ul revine online.

### 3.5 OTA service
- **Depinde de:** 3.1
- **Fișiere noi:** app Django `ota/` cu `Firmware`, `RolloutPlan`, S3-compatible storage pentru artefacte.
- **Acțiune:** rollout staged (canary → 10% → 100%), rollback automat la error rate ridicat.
- **Done when:** o lansare nouă ajunge controlat la device-uri; rollback la depășire de threshold.

---

## Faza 4 — Funcționalități de platformă

Toate sub-pasele sunt **paralelizabile** între ele. Precondiție: Faza 3 încheiată.

### 4.1 Rule engine / scenes / automations
- **Acțiune:** model `Rule(when, conditions, actions)`, evaluator pe stream MQTT (Go consumer separat).
- **Fișiere noi:** `django-bakend/rules/`, `go-iot-platform/cmd/rule-engine/`.

### 4.2 Notifications (FCM/APNs/email/webhooks)
- **Fișiere noi:** `notifications/` Django app + provider integrations.

### 4.3 Open API public + portal developer
- **Acțiune:** versionare `/api/v1/`, OpenAPI spec, generator de keys, portal Sphinx/Redoc.

### 4.4 Observabilitate end-to-end
- **Acțiune:** OpenTelemetry (Django + Go), Prometheus client în ambele, Grafana dashboards, Loki pentru log-uri, alerting.

### 4.5 Mobile SDK + white-label apps
- **Acțiune:** SDK iOS/Android (out of scope ca implementare detaliată aici, dar înregistrat în plan ca dependență de 3.1, 3.3).

### 4.6 Billing & quotas
- **Acțiune:** plan tiers, metering pe scrieri Influx + apeluri API, integrare Stripe.

### 4.7 Audit log per tenant
- **Acțiune:** tabel append-only `audit_log(tenant_id, actor, action, resource, ts)` populat din middleware Django + signal handlers.

---

## Verificare end-to-end

La sfârșitul fiecărei faze, rulează:

1. **Suite automatizat** (CI verde): `pytest django-bakend/`, `go test ./go-iot-platform/...`.
2. **Smoke test integrat** (script `scripts/e2e.sh` sau `e2e.ps1`):
   - Pornește componentele direct: `python manage.py runserver`, `./bin/go-iot-platform`, Kong (`kong start -c kong.conf`), MQTT broker și InfluxDB rulează ca servicii pe gazda existentă.
   - Creează 2 tenanți, 1 user pe fiecare, 1 device pe fiecare (după Faza 1).
   - Publică un mesaj MQTT pe device-ul tenant A (`mosquitto_pub`).
   - Verifică în Influx că punctul are `tenant_id=A` (`influx query`).
   - Cere API-ul Go cu token tenant B pentru device tenant A → așteaptă 403.
   - Cere cu token tenant A pentru device tenant A → așteaptă 200 cu valoarea publicată.
3. **Load test** (după Faza 2): `mqtt-bench` sau `k6` care publică 10k msg/s timp de 10 min; verifică pierderi și lag end-to-end.
4. **Security check** (după Faza 0 și după Faza 3.1): `gitleaks` pentru scan secrete în repo, `pip-audit` pentru dependențe Python, `govulncheck ./...` pentru Go, test penetrare ACL MQTT.

---

## Fișiere critice de modificat (sumar)

| Componentă | Fișier | Faza |
|---|---|---|
| Auth Django | [clients/tokens.py](django-bakend/clients/tokens.py) | 1.3 |
| Modele Django | [clients/models.py](django-bakend/clients/models.py), `tenants/models.py` (nou) | 1.1, 1.2 |
| Views Django | [clients/views.py](django-bakend/clients/views.py) | 1.4, 1.5 |
| Settings Django | [django_backend/settings.py](django-bakend/django_backend/settings.py) | 0.1, 1.1 |
| Go main | [cmd/main.go](go-iot-platform/cmd/main.go) | 0.3, 0.6, 1.8, 2.3, 2.5 |
| Go API | [internal/api/handlers.go](go-iot-platform/internal/api/handlers.go) | 1.7 |
| Go Influx | [internal/influx/client.go](go-iot-platform/internal/influx/client.go) | 0.6, 1.7, 2.7 |
| Go Django client | [internal/django/client.go](go-iot-platform/internal/django/client.go) | 0.2, 1.7 |
| Go middleware | [internal/api/middleware.go](go-iot-platform/internal/api/middleware.go) | 0.4 |
| Kong config | [kong/kong.yaml](kong/kong.yaml) | 0.1, 1.6 |
| Requirements / .gitignore | [django-bakend/requirements.txt](django-bakend/requirements.txt), [.gitignore](.gitignore) | 0.6 |
| CI | `.github/workflows/ci.yml` (nou), `django-bakend/requirements-dev.txt` (nou), `*_test.go`, `clients/tests/` | 0.5 |
