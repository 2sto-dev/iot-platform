# Analiză platformă IoT — stadiu actual vs. țintă (5.000 tenanți / 20.000 device-uri, model Tuya)

> Data analizei: 2026-04-25
> Scope analizat: [django-bakend/](django-bakend/), [go-iot-platform/](go-iot-platform/), [kong/](kong/)

---

## 1. Sumar executiv

Platforma este, în acest moment, un **MVP funcțional la scară mică**, format din trei componente:

1. **Django REST API** — gestionează utilizatori (`Client`) și device-uri ([django-bakend/clients/models.py](django-bakend/clients/models.py))
2. **Serviciu Go** — abonat MQTT global + REST API pentru citire metrici din InfluxDB ([go-iot-platform/cmd/main.go](go-iot-platform/cmd/main.go))
3. **Kong API Gateway** — validare JWT și routing către cele două backend-uri ([kong/kong.yaml](kong/kong.yaml))

**Verdict global:** arhitectura actuală **NU poate susține** ținta de 5.000 tenanți × 20.000 device-uri. Pentru a ajunge la un nivel comparabil cu Tuya, sunt necesare schimbări **fundamentale** la nivelul:
- modelului de date (lipsește complet conceptul de **tenant/organizație**),
- securității per-device (lipsesc credențiale individuale, provisioning, OTA),
- stratului de ingestie (MQTT abonat la `#` + apel HTTP la Django pe fiecare mesaj — nu scalează),
- multi-tenancy în storage (InfluxDB — un singur bucket; MySQL — un singur schema fără partiționare),
- platformei de comenzi/automatizări/scene (inexistentă).

Estimativ, codul actual reprezintă **5–10%** din ceea ce înseamnă o platformă tip Tuya.

---

## 2. Stadiu actual — ce există

### 2.1 Backend Django ([django-bakend/](django-bakend/))

- **Model utilizator custom** `Client` extinde `AbstractUser` cu `prenume` și `telefon` ([clients/models.py:5-10](django-bakend/clients/models.py#L5-L10))
- **Model `Device`**: serial_number unic global, FK către `Client`, 4 tipuri hardcodate (`shelly_em`, `nous_at`, `zigbee_sensor`, `auto_detected`) ([clients/models.py:13-27](django-bakend/clients/models.py#L13-L27))
- **JWT** prin `djangorestframework_simplejwt`, cu issuer custom `"django"` injectat în token pentru ca Kong să-l valideze ([clients/tokens.py:1-12](django-bakend/clients/tokens.py#L1-L12))
- **API REST** minimal: `DeviceViewSet` (CRUD) + endpoint `GET /api/devices/<username>/` ([clients/views.py:13-49](django-bakend/clients/views.py#L13-L49))
- **Topic templates** mapate per tip de device ([clients/topic_templates.py](django-bakend/clients/topic_templates.py))
- **Admin Django** cu queryset filtrat după `request.user` pentru non-superuser ([clients/admin.py:55-59](django-bakend/clients/admin.py#L55-L59))
- **DB**: MySQL configurat exclusiv din `.env` ([django_backend/settings.py:58-67](django-bakend/django_backend/settings.py#L58-L67))

### 2.2 Serviciu Go ([go-iot-platform/](go-iot-platform/))

- **MQTT subscriber** care se loghează ca **superuser Django** la pornire pentru a obține un JWT și a citi toate device-urile ([cmd/main.go:41-43](go-iot-platform/cmd/main.go#L41-L43), [internal/django/client.go:53-75](go-iot-platform/internal/django/client.go#L53-L75))
- Se abonează la **fiecare topic** primit din Django **și** la wildcard `#` (toate topicurile broker-ului) ([cmd/main.go:97-115](go-iot-platform/cmd/main.go#L97-L115))
- **Auto-discovery**: la primirea unui mesaj de la un device necunoscut, îl înregistrează automat în Django cu `ClientID=1` hardcodat ([cmd/main.go:163-184](go-iot-platform/cmd/main.go#L163-L184))
- **Parsing payload** pe topic suffix (Shelly emeter, NousAT STATE/SENSOR, Zigbee2MQTT, generic) și scriere în InfluxDB ([cmd/main.go:187-287](go-iot-platform/cmd/main.go#L187-L287))
- **API Go**: un singur endpoint `GET /go/metrics/{device}/{field}` care extrage `username` din JWT, întreabă Django dacă userul are dreptul la device, citește ultima valoare din Influx ([internal/api/handlers.go:50-112](go-iot-platform/internal/api/handlers.go#L50-L112))
- **Influx query** fix pe `range: -5m` cu filtru pe measurement `devices` ([internal/influx/client.go:24-29](go-iot-platform/internal/influx/client.go#L24-L29))

### 2.3 Kong ([kong/kong.yaml](kong/kong.yaml))

- DB-less mode cu un singur consumer (`django-users`) și un singur `jwt_secret` cu `key=django` și `secret=123456789` ([kong/kong.yaml:9-16](kong/kong.yaml#L9-L16))
- Trei servicii: `django-auth` (login/refresh, fără JWT), `django-devices` (cu JWT), `go-api` (cu JWT)
- Plugin `prometheus` global activat
- File-log activat pe ruta de devices

---

## 3. Puncte tari

| # | Aspect | Detaliu |
|---|--------|---------|
| 1 | **Separare clară a responsabilităților** | Django pentru control plane (utilizatori/device-uri), Go pentru data plane (MQTT + Influx). Pattern corect pentru o platformă IoT. |
| 2 | **API Gateway centralizat** | Kong validează JWT-ul în fața ambelor servicii, ceea ce simplifică partea de auth la nivel de microserviciu. |
| 3 | **Storage adecvat pentru telemetrie** | InfluxDB este alegere bună pentru time-series (vs. a îndesa metrici în MySQL). |
| 4 | **Custom user model setat de la început** | `AUTH_USER_MODEL = "clients.Client"` — corect făcut din migrația 0001, evită refactor masiv ulterior. |
| 5 | **Topic templates configurabile** | Mapare device_type → topics într-un singur loc, ușor de extins. |
| 6 | **JWT cu issuer claim** | Folosit ca `key_claim_name` în Kong — pattern curat pentru mai mulți emitenți de token. |
| 7 | **Auto-discovery device-uri** | Conceptul există (deși implementarea e problematică, vezi §4). |
| 8 | **Configurare via `.env` / `decouple`** | Secretele nu sunt în cod (cu o excepție majoră, Kong — vezi §4). |

---

## 4. Puncte slabe (în ordinea criticității)

### 4.1 CRITIC — Lipsește complet conceptul de tenant

**Problema:** Modelul de date este `Client (user) → Device`. Nu există entitate `Tenant` / `Organization` / `Workspace`. Tuya organizează:

```
Tenant (company) → Apps → Users → Homes → Rooms → Devices
                       └─ Roles/Permissions
                       └─ API keys / Webhooks
                       └─ Quotas / Billing
```

**Impact pentru 5.000 tenanți:**
- Un tenant nu poate avea mai mulți utilizatori cu roluri diferite (admin, instalator, end-user, suport).
- Nu există izolare logică între tenanți — totul partajează același namespace de `serial_number` global unique ([clients/models.py:22](django-bakend/clients/models.py#L22)).
- Nu se poate factura pe tenant, nu se pot aplica quote, nu se poate suspenda un tenant.

**Necesar:** model `Tenant`, `Membership` (User × Tenant × Role), tabela `Device` cu FK către `Tenant`, nu către user. Toate query-urile devin `WHERE tenant_id = ?`. Indexare obligatorie pe `tenant_id`.

### 4.2 CRITIC — Ingestia MQTT nu scalează

**Problema:** Serviciul Go are mai multe defecte care se cumulează:

1. **Abonare la `#`** ([cmd/main.go:108](go-iot-platform/cmd/main.go#L108)) — primește **toate** mesajele de pe broker. La 20.000 device-uri × frecvența lor de raportare, asta poate însemna mii–zeci de mii de msg/sec direct într-un singur proces.
2. **Apel HTTP către Django pe fiecare mesaj** ([cmd/main.go:154](go-iot-platform/cmd/main.go#L154)) — `GetAllDevices()` făcut sincron din `handleMessage`. Latență + presiune masivă pe Django.
3. **Un singur worker** — gorutina de message handler nu are pool, fără backpressure, fără queue.
4. **Scriere blocking în Influx** ([cmd/main.go:48](go-iot-platform/cmd/main.go#L48)) cu `WriteAPIBlocking` — fiecare punct = un round-trip HTTP. Influx are batch API care e ordine de mărime mai eficient.
5. **Auto-register sincron pe message path** — orice mesaj de la un device nou face un POST la Django ([cmd/main.go:179](go-iot-platform/cmd/main.go#L179)).
6. **Hardcoded `ClientID: 1`** ([cmd/main.go:178](go-iot-platform/cmd/main.go#L178)) — toate device-urile auto-descoperite ajung la primul user din DB. **Catastrofal în multi-tenant.**
7. **Process unic** — fără replicas, fără MQTT shared subscriptions (`$share/group/topic`). La crash se pierd date.

**Necesar:** broker MQTT clusterizat (EMQX / VerneMQ / HiveMQ Enterprise), shared subscriptions, pool de consumeri, cache local pentru autorizare device→tenant (Redis), batch writes în Influx (sau Kafka → ingestor → Influx), back-pressure, dead-letter queue.

### 4.3 CRITIC — Lipsește autentificarea per device

**Problema:** Nu există niciun mecanism prin care un device să se autentifice individual. Brokerul MQTT are un singur user/pass partajat (`MQTT_USER`/`MQTT_PASS` din env). Orice device care cunoaște parola broker-ului poate publica pe **orice topic**, inclusiv pe topicuri ale altor tenanți.

**Necesar (model Tuya):**
- Per-device credentials (deviceId + deviceSecret) emise la provisioning.
- ACL MQTT per device, restricționând topicurile la `tenants/{tenant_id}/devices/{device_id}/#`.
- Activation flow: device produs cu un token de fabrică → activare → primește credențiale runtime.
- Rotation de chei.
- Suport pentru certificate X.509 / mTLS pentru device-uri high-security.

### 4.4 CRITIC — Secret management

- **JWT secret hardcodat în Kong:** `secret: "123456789"` ([kong/kong.yaml:16](kong/kong.yaml#L16)). Trebuie sincronizat cu Django și pus în vault (HashiCorp Vault / AWS Secrets Manager / sealed secrets).
- **Superuser Django folosit de Go-ul de ingest** ([cmd/main.go:41](go-iot-platform/cmd/main.go#L41)) — single point of compromise. Orice leak al `DJANGO_SUPERUSER`/`DJANGO_SUPERPASS` dă acces total. Necesar: service account dedicat cu permisiuni minime, sau autentificare prin mTLS între servicii.
- Nu există rotație de secrete, nu există revocation list pentru JWT.
- `ACCESS_TOKEN_LIFETIME = 100 minute` ([settings.py:111](django-bakend/django_backend/settings.py#L111)) — agresiv, dar fără refresh token revocation pe logout.

### 4.5 ÎNALT — Topology MQTT inadecvată

Topicurile actuale sunt **orientate pe vendor**, nu pe tenant:

```
shellies/{serial}/emeter/0/power
tele/{serial}/STATE
zigbee2mqtt/{serial}
```

Tuya / orice platformă serioasă folosește un namespace propriu, **tenant-aware**:

```
tenants/{tenant_id}/devices/{device_id}/up/telemetry
tenants/{tenant_id}/devices/{device_id}/down/cmd
tenants/{tenant_id}/devices/{device_id}/up/event
tenants/{tenant_id}/devices/{device_id}/up/state
```

Pentru integrarea cu device-uri off-the-shelf (Shelly, NousAT) ai nevoie de **MQTT bridge / translator** care normalizează topicurile către schema internă. Asta lipsește.

### 4.6 ÎNALT — Lipsă control plane pentru device-uri (downlink)

În cod **nu există nicio cale de a trimite o comandă** către un device. Tot fluxul e doar **uplink** (device → cloud → Influx). Tuya, în schimb, oferă:
- Comenzi de control (on/off, set thermostat 22°C etc.)
- Confirmări (ACK)
- OTA updates
- Configurare runtime
- Scene & automations
- Pairing flow (QR / Bluetooth / SoftAP)

Toate acestea **lipsesc complet**.

### 4.7 ÎNALT — Lipsă multi-tenancy în storage

- **InfluxDB:** un singur bucket `INFLUX_BUCKET`, un singur org. La 20.000 device-uri × telemetrie → un singur bucket devine atât un risc de izolare cât și un hotspot. Necesar: bucket per tenant (sau cel puțin tag obligatoriu `tenant_id` și retention policies separate), retenție configurabilă pe tenant pentru tier-uri de plan.
- **MySQL:** un singur schema, fără sharding. La 5.000 tenanți × N users × M devices, query-urile cu `WHERE tenant_id = ?` trebuie să aibă indecși acoperitori. Va fi nevoie cel puțin de partitioning pe `tenant_id` la tabele mari (telemetrie, audit, evenimente).
- **`serial_number` unique global** ([clients/models.py:22](django-bakend/clients/models.py#L22)) — două device-uri cu același serial de la doi vendori diferiți pe doi tenanți distincți se ciocnesc. Trebuie unique compus `(tenant_id, serial_number)`.

### 4.8 ÎNALT — Lipsă observabilitate end-to-end

- Există plugin-ul `prometheus` în Kong, dar **nu apare** instrumentare în Django (nu sunt exportate metrici Django) sau în Go (nu folosește `prometheus/client_golang`).
- Nu există tracing distribuit (OpenTelemetry).
- Logarea: în Go se scrie într-un fișier local + stdout ([cmd/main.go:36-37](go-iot-platform/cmd/main.go#L36-L37)). Lipsește log aggregation (Loki / ELK).
- Nu există dashboards, alerting, SLO-uri.

### 4.9 ÎNALT — Lipsă teste, CI/CD, automatizare

- `clients/tests.py` are 3 linii goale ([clients/tests.py](django-bakend/clients/tests.py)).
- Nu există teste în Go (`*_test.go` lipsă).
- Nu există GitHub Actions / GitLab CI / Jenkins.
- Nu există Dockerfile / docker-compose / Helm chart / Terraform.
- Deploy-ul actual este implicit manual.

Pentru o platformă cu 5.000 tenanți, asta este **inacceptabil** — o regresie netestată afectează producția pentru toți.

### 4.10 ÎNALT — CORS „allow all"

`Access-Control-Allow-Origin: *` ([go-iot-platform/internal/api/middleware.go:10](go-iot-platform/internal/api/middleware.go#L10)). Pentru o platformă cu apps/dashboards multiple ale tenanților, CORS trebuie configurat per-tenant cu listă de domenii permise.

### 4.11 ÎNALT — Lipsă Redis / cache layer

Toate apelurile Go → Django trec prin HTTP + DB ([cmd/main.go:154](go-iot-platform/cmd/main.go#L154), [api/handlers.go:76](go-iot-platform/internal/api/handlers.go#L76)). Pentru 20k device-uri, trebuie un cache (Redis) cu invalidare la modificări — altfel Django devine bottleneck.

### 4.12 MEDIU — Funcționalități platformă lipsă

| Funcționalitate Tuya | Stadiu |
|----------------------|--------|
| Pairing wizard (QR, BLE, SoftAP) | ❌ |
| Scene & Automations | ❌ |
| Sharing devices între users | ❌ |
| Group control (toate becurile dintr-o cameră) | ❌ |
| Energy statistics dashboard | ❌ (există date în Influx, nu există agregări) |
| Notifications (push / email / SMS) | ❌ |
| Webhooks pentru integrare externă | ❌ |
| Mobile SDK / app | ❌ |
| Open API pentru tenanți (PaaS) | ❌ |
| OTA firmware management | ❌ |
| Device timeline / event log | ❌ |
| Audit log per tenant | ❌ |
| RBAC fin (peste superuser/normal) | ❌ |
| Localizare (i18n) | ❌ (UTC, en-us) |
| Plan & billing | ❌ |
| Status page / health checks | ❌ |

### 4.13 MEDIU — Probleme punctuale de cod

- `WriteAPIBlocking` blocant ([cmd/main.go:48](go-iot-platform/cmd/main.go#L48)) — folosește `WriteAPI` async cu batch.
- `strings.Title` deprecated ([cmd/main.go:197](go-iot-platform/cmd/main.go#L197)) — folosește `golang.org/x/text/cases`.
- `select {}` care blochează main fără cleanup ([cmd/main.go:121](go-iot-platform/cmd/main.go#L121)) — fără signal handling pentru graceful shutdown.
- Embed log inițial cu `//go:embed go_meeter.log` ([cmd/main.go:25-26](go-iot-platform/cmd/main.go#L25-L26)) — un fișier de log statică în binar nu are rost; va divergui de la realitate.
- Fix `range: -5m` în Influx ([internal/influx/client.go:26](go-iot-platform/internal/influx/client.go#L26)) — dacă device-ul nu a raportat în 5 min → 500 error. Necesar parametrizat și fallback explicit.
- `requirements.txt` are encoding UTF-16 LE BOM (vizibil din spațiile dintre caractere) — nu va fi parsat corect pe Linux fără atenție.
- `clients/views.py:15` `queryset = Device.objects.all()` la nivel de class — DRF îl folosește pentru `basename` și pentru `get_object()`. Nu e bug, dar combinat cu `get_queryset()` poate scăpa accidental obiecte dacă cineva schimbă clasa.

### 4.14 MEDIU — Hardcodări

- IP-uri hardcodate în [kong/kong.yaml](kong/kong.yaml): `172.16.0.105:8000`, `172.16.0.105:8080`. Trebuie service discovery (DNS interior / Kong upstreams cu health checks).
- `baseURL` Django default `http://172.16.0.105:8000/api` ([go-iot-platform/internal/django/client.go:23](go-iot-platform/internal/django/client.go#L23)).
- `ClientID: 1` hardcodat în auto-register ([cmd/main.go:178](go-iot-platform/cmd/main.go#L178)).

---

## 5. Gap-uri specifice pentru a deveni „ca Tuya"

| Domeniu | Gap | Efort estimat |
|---------|-----|---------------|
| **Tenancy** | Model complet Tenant/Org/Membership/Roles | 3–4 săptămâni |
| **Provisioning** | QR, BLE, SoftAP, activation server, device certs | 6–8 săptămâni |
| **Device SDK** | C/C++ embedded SDK + iOS/Android SDK | 12+ săptămâni |
| **Mobile apps** | iOS + Android white-label | 16+ săptămâni |
| **Cloud Open API** | API public versionat + portal developer | 6 săptămâni |
| **Rule engine** | Scenes/automations/triggers cross-device | 8 săptămâni |
| **OTA** | Repository firmware + rollout staged + fallback | 4 săptămâni |
| **Notifications** | FCM/APNs + email + SMS + webhooks | 3 săptămâni |
| **Billing** | Plan tiers + metering + Stripe | 4 săptămâni |
| **Compliance** | GDPR, SOC2, ISO 27001, audit logs | continuu |
| **Multi-region** | Dispatcher + replication + data residency | 8+ săptămâni |
| **Edge / local control** | LAN protocol pentru control fără cloud | 6 săptămâni |

---

## 6. Recomandări — pași în ordine de prioritate

### Faza 0 — Stabilizare (urgent, înainte de orice scaling)

1. Scoate `secret: "123456789"` din [kong/kong.yaml](kong/kong.yaml) și mută-l în vault.
2. Înlocuiește `ClientID: 1` hardcodat cu un mecanism corect ([cmd/main.go:178](go-iot-platform/cmd/main.go#L178)) — minim, refuză înregistrarea în loc să o atribui aiurea.
3. Service account dedicat pentru Go (în loc de superuser).
4. Schimbă CORS din `*` în listă explicită.
5. Adaugă `.gitignore`, `Dockerfile`, `docker-compose.yml` pentru dev local reproductibil.

### Faza 1 — Refactor multi-tenant (fundație)

1. **Introduce modelul `Tenant`** și `Membership` (User × Tenant × Role) în Django.
2. Toate tabelele devin **tenant-scoped** — adaugă `tenant_id` cu index, schimbă unique pe `(tenant_id, serial_number)`.
3. Middleware Django + queryset manager care injectează automat `tenant_id` din JWT claim.
4. JWT-ul include `tenant_id` și `roles`.
5. RBAC explicit: `TENANT_ADMIN`, `TENANT_OPERATOR`, `TENANT_VIEWER`, `DEVICE_INSTALLER`.

### Faza 2 — Ingest scalabil

1. Schimbă brokerul cu **EMQX/VerneMQ** clusterizat, cu **ACL per device**.
2. **Topicuri tenant-scoped**: `tenants/{tid}/devices/{did}/up/...`.
3. Servicul Go devine **stateless worker** cu `$share/...` shared subscription, replicabil.
4. Cache device→tenant în **Redis** cu invalidare push de la Django.
5. Influx batch writes (`WriteAPI` cu flush la 1s/5k puncte).
6. Opțional: introducere **Kafka/NATS** ca buffer între MQTT consumer și Influx writer.

### Faza 3 — Control plane device

1. Provisioning service: emitere device credentials + activation token.
2. Downlink command path: API → Kafka → MQTT publish pe `down/cmd`.
3. Status & shadow (last reported state, desired state).
4. OTA service.

### Faza 4 — Funcționalități platform

Rule engine, scenes, automations, sharing, notifications, mobile SDK, open API. În paralel: observabilitate (OTel + Grafana + Loki + alerts), CI/CD, IaC.

---

## 7. Estimare capacitate actuală vs. țintă

| Metrică | Capacitate actuală (estimat) | Țintă | Factor lipsă |
|---------|------------------------------|-------|--------------|
| Tenanți | 1 (concept inexistent) | 5.000 | 5.000× |
| Device-uri active | ~50–200 (limită MQTT single-process + Influx blocking writes) | 20.000 | 100×–400× |
| Mesaje/secundă | ~50–200 msg/s sustenabile | 10.000+ msg/s | 50×–200× |
| Disponibilitate | Single-instance, no failover | 99.9%+ | redesign infra |
| Time-to-onboard tenant | N/A | < 5 min self-service | redesign UX |

---

## 8. Concluzie

Platforma actuală este un **proof of concept funcțional** care demonstrează corect ideea: separarea control plane (Django) de data plane (Go), gateway centralizat (Kong), time-series storage (Influx). Pattern-urile de bază sunt sănătoase și se pot construi peste ele.

**Dar drumul până la „ca Tuya, pentru 5.000 tenanți și 20k device-uri" este lung.** Recomandarea concretă este să nu se încerce o tranziție „big-bang" — fazele 0 și 1 (stabilizare + refactor multi-tenant) trebuie făcute **înaintea** oricărei optimizări de performanță, pentru că orice scaling al arhitecturii actuale (single-tenant implicit) va deveni datorie tehnică pe care o vei plăti dublu mai târziu.

Prioritatea #1 absolută: **introducerea modelului `Tenant` și a izolării per-tenant la toate nivelurile (DB, MQTT, Influx, JWT, Kong consumer)**. Fără asta, orice altă lucrare e construită pe nisip.
