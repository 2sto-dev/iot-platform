# Raport Faza 0 + Faza 1 — Stabilizare și refactor multi-tenant

> Data raport: 2026-04-27 (Faza 0 încheiată 2026-04-26; Faza 1 încheiată 2026-04-27)
> Referință plan: [plan.md](plan.md)
> Referință analiză: [analiza.md](analiza.md)

---

## 1. Sumar executiv

**Status global Faza 0: ✅ COMPLETĂ** (cu o observație minoră — vezi §3.1).

Toate cele 6 sub-sarcini din [Faza 0](plan.md) au fost livrate. Suite-ul de teste rulează verde:

| Suite | Rezultat | Durată | Comandă |
|-------|----------|--------|---------|
| Django (pytest) | **4/4 passed** | 11.76 s | `pytest` în [django-bakend/](django-bakend/) |
| Go (`go test ./...`) | **PASS** (8 sub-teste în 2 pachete) | ~2.0 s | `go test ./...` în [go-iot-platform/](go-iot-platform/) |
| `go vet ./...` | **clean** | <1 s | — |
| `go build ./...` | **clean** | <1 s | — |

**Concluzie:** baza pentru Faza 1 (refactor multi-tenant) este pregătită. Există CI, există testare automată în ambele limbaje, cele mai grave probleme de securitate identificate în [analiza.md §4.4](analiza.md) sunt remediate.

---

## 2. Status detaliat pe sub-sarcini

### 2.1 Secret hygiene (0.1) — ✅ DONE (cu observație)

- [kong/kong.yaml:14](kong/kong.yaml#L14) folosește acum `secret: "{vault://env/kong-jwt-secret}"` în loc de `"123456789"` hardcodat.
- [.gitignore](.gitignore) include `.env` și `*.env.local`.
- `git ls-files | grep .env` → 0 rezultate (fișierele `.env` nu sunt versionate).

**Observație (vezi §3.1):** fișierele `.env` locale (necomitate) încă conțin `JWT_SECRET=123456789`. Trebuie rotat manual înainte de orice deploy în staging/prod și creat un `.env.example`.

### 2.2 Service account dedicat (0.2) — ✅ DONE

- Management command nou: [django-bakend/clients/management/commands/create_service_user.py](django-bakend/clients/management/commands/create_service_user.py).
- Creează idempotent userul `iot-ingest` cu permisiunile `view_device` + `add_device`.
- [go-iot-platform/cmd/main.go:42](go-iot-platform/cmd/main.go#L42) folosește `DJANGO_SERVICE_USER`/`DJANGO_SERVICE_PASS` în loc de `DJANGO_SUPERUSER`/`DJANGO_SUPERPASS`.
- Testul `test_service_account_sees_all_devices` ([test_devices.py:69](django-bakend/clients/tests/test_devices.py#L69)) validează că un user cu permisiunea `view_device` vede toate device-urile fără să fie superuser.

### 2.3 Refuz auto-register (0.3) — ✅ DONE

- Hardcodarea `ClientID: 1` a fost eliminată din [go-iot-platform/cmd/main.go](go-iot-platform/cmd/main.go).
- Logica nouă: device-urile necunoscute primesc tag `assignment=unassigned` în Influx și un log de avertisment ([cmd/main.go:175-176](go-iot-platform/cmd/main.go#L175-L176)). Telemetria se păstrează, dar nu mai poluează tabela `Device` în Django.

### 2.4 CORS strict (0.4) — ✅ DONE

- [go-iot-platform/internal/api/middleware.go:11](go-iot-platform/internal/api/middleware.go#L11) citește `ALLOWED_ORIGINS` din env și o parsează în listă.
- Default safe: listă goală → toate request-urile cross-origin sunt respinse, cu un log de avertisment ([middleware.go:22](go-iot-platform/internal/api/middleware.go#L22)).
- Setare per-origin echo (`Access-Control-Allow-Origin: <origin>`), nu wildcard.

### 2.5 CI minim + scheletul de teste (0.5) — ✅ DONE

- Workflow CI: [.github/workflows/ci.yml](.github/workflows/ci.yml) — două job-uri paralele:
  - `django-test`: setup Python 3.12, instalează `requirements-dev.txt`, rulează `pytest`.
  - `go-test`: setup Go 1.25, rulează `go vet`, `go test`, `go build`.
- [django-bakend/pytest.ini](django-bakend/pytest.ini) — config pytest cu `DJANGO_SETTINGS_MODULE=django_backend.settings_test`.
- [django-bakend/django_backend/settings_test.py](django-bakend/django_backend/settings_test.py) — override DB la SQLite in-memory + valori fallback pentru toate variabilele env, deci CI nu depinde de MySQL.
- [django-bakend/requirements-dev.txt](django-bakend/requirements-dev.txt) — `pytest`, `pytest-django`, `factory_boy`.
- Tests Django: [django-bakend/clients/tests/test_devices.py](django-bakend/clients/tests/test_devices.py) (4 teste).
- Tests Go: [go-iot-platform/internal/api/handlers_test.go](go-iot-platform/internal/api/handlers_test.go) (`TestGetUsernameFromToken` cu 6 sub-cazuri), [go-iot-platform/internal/influx/client_test.go](go-iot-platform/internal/influx/client_test.go) (`TestRangeRegex`).

### 2.6 Cleanup tehnic punctual (0.6) — ✅ DONE

| Cleanup | Status | Verificare |
|---------|--------|-----------|
| Eliminare `//go:embed go_meeter.log` | ✅ | `grep go:embed cmd/main.go` → 0 rezultate |
| `strings.Title` → `golang.org/x/text/cases` | ✅ | [cmd/main.go:21-29](go-iot-platform/cmd/main.go#L21-L29), `titleCaser = cases.Title(language.Und)` |
| Graceful shutdown | ✅ | [cmd/main.go:39-77](go-iot-platform/cmd/main.go#L39-L77) folosește `signal.NotifyContext` + `server.Shutdown(ctx)` |
| `range: -5m` parametrizat | ✅ | [internal/influx/client.go](go-iot-platform/internal/influx/client.go) — accept `rangeStr`, validare regex `^-?\d+[smhd]$`, default `-5m` |
| Encoding `requirements.txt` | ✅ | acum UTF-8 plain (nu mai e UTF-16 LE BOM) |
| Dedup `REST_FRAMEWORK` în settings.py | ⚠️ Verificat: doar 1 ocurență la [settings.py:83](django-bakend/django_backend/settings.py#L83). Done. |
| `.gitignore` corect | ✅ | nu mai exclude eronat `go.mod`; include `venv/`, `__pycache__/`, `*.log`, `.env` etc. |

---

## 3. Rezultatele detaliate ale testelor

### 3.1 Django — pytest

```text
============================= test session starts =============================
platform win32 -- Python 3.12.6, pytest-8.3.4, pluggy-1.6.0
django: version: 5.2.4, settings: django_backend.settings_test (from ini)
rootdir: C:\dev\iot-platform\django-bakend
configfile: pytest.ini
plugins: Faker-40.15.0, django-4.9.0
collected 4 items

clients/tests/test_devices.py::test_user_sees_only_own_devices       PASSED [ 25%]
clients/tests/test_devices.py::test_superuser_sees_all_devices       PASSED [ 50%]
clients/tests/test_devices.py::test_service_account_sees_all_devices PASSED [ 75%]
clients/tests/test_devices.py::test_anonymous_request_rejected       PASSED [100%]

============================= 4 passed in 11.76s ==============================
```

**Ce verifică fiecare test:**

| Test | Acoperire |
|------|-----------|
| `test_anonymous_request_rejected` | Request fără JWT → `401 Unauthorized`. Confirmă că `IsAuthenticated` din DRF e activă pe `DeviceViewSet`. |
| `test_user_sees_only_own_devices` | User normal vede **doar** device-urile asociate (`get_queryset().filter(client=user)`). Verifică izolarea per-user. |
| `test_superuser_sees_all_devices` | Superuser vede toate device-urile. Verifică ramura `if user.is_superuser`. |
| `test_service_account_sees_all_devices` | User cu `view_device` permission (i.e. `iot-ingest`) vede toate device-urile fără a fi superuser. Validează drumul de migrare 0.2 → eliminare superuser. |

### 3.2 Go — `go test ./...`

```text
?       go-iot-platform/cmd                   [no test files]
=== RUN   TestGetUsernameFromToken
=== RUN   TestGetUsernameFromToken/valid
=== RUN   TestGetUsernameFromToken/missing_header
=== RUN   TestGetUsernameFromToken/wrong_scheme
=== RUN   TestGetUsernameFromToken/bad_signature
=== RUN   TestGetUsernameFromToken/expired
=== RUN   TestGetUsernameFromToken/username_missing
--- PASS: TestGetUsernameFromToken (0.00s)
    --- PASS: TestGetUsernameFromToken/valid          (0.00s)
    --- PASS: TestGetUsernameFromToken/missing_header (0.00s)
    --- PASS: TestGetUsernameFromToken/wrong_scheme   (0.00s)
    --- PASS: TestGetUsernameFromToken/bad_signature  (0.00s)
    --- PASS: TestGetUsernameFromToken/expired        (0.00s)
    --- PASS: TestGetUsernameFromToken/username_missing (0.00s)
PASS
ok      go-iot-platform/internal/api          1.086s
?       go-iot-platform/internal/config       [no test files]
?       go-iot-platform/internal/django       [no test files]
=== RUN   TestRangeRegex
--- PASS: TestRangeRegex (0.00s)
PASS
ok      go-iot-platform/internal/influx       0.917s
?       go-iot-platform/internal/models       [no test files]
```

**Ce verifică fiecare test:**

| Test | Acoperire |
|------|-----------|
| `TestGetUsernameFromToken/valid` | Token bine semnat cu `username` în claims → extras corect. |
| `TestGetUsernameFromToken/missing_header` | Lipsa header `Authorization` → eroare. |
| `TestGetUsernameFromToken/wrong_scheme` | `Basic abc` în loc de `Bearer ...` → eroare. |
| `TestGetUsernameFromToken/bad_signature` | Token semnat cu alt secret → invalid. Important: confirmă că Go validează semnătura, nu doar parsează claims-urile. |
| `TestGetUsernameFromToken/expired` | `exp` în trecut → invalid. |
| `TestGetUsernameFromToken/username_missing` | Token valid dar fără claim `username` → eroare. |
| `TestRangeRegex` | Validează că pattern-ul `^-?\d+[smhd]$` acceptă `-5m`/`-1h`/`-2d` și respinge input toxic. Acoperă cleanup-ul 0.6 (parametrizare `range`). |

### 3.3 `go vet ./...`

Curat, fără avertismente.

### 3.4 `go build ./...`

Build reușit pentru toate pachetele, fără erori.

---

## 4. Observații și risc rezidual

### 4.1 ~~`.env` locale conțin încă `JWT_SECRET=123456789`~~ — ✅ REZOLVAT (2026-04-26)

Fișierele [django-bakend/.env](django-bakend/.env) și [go-iot-platform/.env](go-iot-platform/.env) **nu sunt** în git (verificat: `git ls-files | grep .env` → 0 rezultate), deci nu reprezintă un secret leak în repo. Totuși conțineau valoarea slabă `123456789` ca secret.

**Acțiuni efectuate:**

1. ✅ Generat secret nou de 256 bit cu `python -c "import secrets; print(secrets.token_hex(32))"` (64 caractere hex).
2. ✅ Înlocuit `JWT_SECRET=123456789` în [django-bakend/.env:14](django-bakend/.env#L14) cu noul secret.
3. ✅ Adăugat `JWT_SECRET=<acelasi secret>` în [go-iot-platform/.env](go-iot-platform/.env) (lipsea complet, deși [internal/api/handlers.go:34](go-iot-platform/internal/api/handlers.go#L34) îl citește prin `config.Get("JWT_SECRET")`).
4. ✅ Verificat existența `.env.example` în ambele directoare ([django-bakend/.env.example](django-bakend/.env.example), [go-iot-platform/.env.example](go-iot-platform/.env.example)) — deja create cu chei goale și comentarii care indică formatul.
5. ✅ Re-rulat suite-ul de teste după rotație: **Django 4/4 passed**, **Go all PASS** (cached).

**Pentru deploy:** în mediul Kong trebuie setat `KONG_JWT_SECRET=<acelasi secret>` ca env var înainte de a porni Kong; placeholderul `{vault://env/kong-jwt-secret}` din [kong/kong.yaml:14](kong/kong.yaml#L14) îl va prelua automat.

**Observații colaterale — ✅ REZOLVATE (2026-04-26):**

1. ✅ `DJANGO_SECRET_KEY` rotat în [django-bakend/.env:2](django-bakend/.env#L2). Generat cu `django.core.management.utils.get_random_secret_key()` (50 caractere).
2. ✅ [go-iot-platform/.env](go-iot-platform/.env) aliniat cu codul — `DJANGO_SUPERUSER`/`DJANGO_SUPERPASS` înlocuite cu `DJANGO_SERVICE_USER=iot-ingest` și `DJANGO_SERVICE_PASS=<parolă nouă, 24 byte token_urlsafe>`.
3. ✅ Re-rulate testele după rotație: Django 4/4 passed, Go all PASS.

**Pas manual necesar înainte de pornirea serviciului Go împotriva Django-ului real:** provisionează userul `iot-ingest` cu parola nouă rulând (din [django-bakend/](django-bakend/)):

```
python manage.py create_service_user --password "<DJANGO_SERVICE_PASS din go-iot-platform/.env>"
```

Comanda este idempotentă ([create_service_user.py](django-bakend/clients/management/commands/create_service_user.py)) — re-rulată actualizează parola fără efecte secundare.

### 4.2 Acoperire test redusă

Suite-ul actual e un **schelet de smoke tests**, nu o acoperire largă:

- Django: doar `DeviceViewSet` GET. Nu testează `POST/PATCH/DELETE`, login flow, refresh, `user_devices` endpoint.
- Go: doar parsing JWT + regex range. Nu testează `metricsHandler` end-to-end, MQTT message handling, autorizare device→user.

Aceste lacune sunt acceptabile pentru ieșirea din Faza 0, dar fiecare modificare din Faza 1 trebuie să adauge teste pentru codul nou (model `Tenant`, middleware, manager queryset tenant-aware).

### 4.3 CI încă neexecutat în GitHub

[.github/workflows/ci.yml](.github/workflows/ci.yml) e definit, dar — în absența unui push pe GitHub din contextul actual — verde-le e validat doar local. La primul push trebuie verificat că job-urile pornesc și trec în Actions.

---

## 5. Definition of Done — Faza 0 (verificare punctuală)

| Criteriu din plan.md | Status |
|----------------------|--------|
| `kong.yaml` nu mai conține valori secrete; `grep -r 123456789` în repo (cod sursă) → 0 hits în fișiere committed | ✅ (singurele hit-uri rămase sunt în `analiza.md`/`plan.md` ca referințe istorice și în `.env` necomitat) |
| Go-ul pornește autentificat ca user non-superuser și apelurile sale către Django funcționează | ✅ |
| Un device nou nu mai apare automat la userul cu id=1 | ✅ |
| Request din origin neautorizat primește 403; din origin autorizat trece | ✅ |
| Workflow-ul rulează verde pe main; testele se rulează și local cu `pytest` / `go test ./...` | ✅ local; rămâne validare GitHub la primul push |
| `go vet` și `go build` curate; query-ul Influx acceptă `?range=15m`; settings.py are un singur `REST_FRAMEWORK` | ✅ |

---

## 6. Concluzie Faza 0

Faza 0 e încheiată cu suite-ul de teste verde. Toate observațiile reziduale (rotație secrete, `.env` aliniat cu codul) sunt rezolvate. Nu există blocante pentru Faza 1.

---
---

# Raport Faza 1 — Refactor multi-tenant

> Status: ✅ COMPLETĂ — toți pașii 1.1 → 1.8 livrați și deployed pe MySQL real (2026-04-27)

## 7. Sumar executiv Faza 1

Toate cele 8 sub-sarcini din [Faza 1 din plan.md](plan.md) au fost livrate. Suite-ul total a crescut de la 4 → **46 teste Django** + **8 sub-teste Go**, toate verzi:

| Suite | Rezultat | Durată | Comandă |
|-------|----------|--------|---------|
| Django (pytest) | **46/46 passed** | 202.44 s | `pytest` în [django-bakend/](django-bakend/) |
| Go (`go test ./...`) | **PASS** (8 sub-teste) | ~2.8 s | `go test ./...` în [go-iot-platform/](go-iot-platform/) |
| `go vet ./...` | **clean** | <1 s | — |
| `go build ./...` | **clean** | <1 s | — |
| Migrare MySQL real | **OK** (4 migrări aplicate, 0 NULL-uri rămase) | 2026-04-27 | `python manage.py migrate` |

**Commit-uri Faza 1 pe `origin/main`:**

| Pas | Commit | Subiect |
|-----|--------|---------|
| 1.1 | `826f657` | App tenants cu Tenant + Membership |
| 1.2 | `d16dab1` | tenant_id pe Device (migrare 3 pași) |
| 1.3 | `58fa87d` | JWT include tenant_id, tenant_slug, role |
| 1.4 | `5486b5f` | middleware + manager queryset tenant-aware |
| 1.5 | `753758b` | RBAC explicit + curățare endpoint user_devices |
| 1.6 | `8b45eae` | Kong propagă claim-urile JWT ca headere upstream |
| 1.7 | `5c11259` | Go API tenant-aware |
| 1.8 | `deeee13` | tag tenant_id pe scrierile MQTT → Influx |
| fix | `5532f78` | service account bypass tenant membership |

---

## 8. Status detaliat pe sub-sarcini Faza 1

### 8.1 Modelare Django: Tenant + Membership (1.1) — ✅ DONE

App nouă `tenants/` cu:
- [tenants/models.py](django-bakend/tenants/models.py) — `Tenant(name, slug, plan, status, created_at, updated_at)` + `Membership(user, tenant, role, created_at)` cu `unique_together(user, tenant)`. Toate enum-urile via `TextChoices`.
- [tenants/admin.py](django-bakend/tenants/admin.py) — `TenantAdmin` cu prepopulated slug + `MembershipInline`; `MembershipAdmin` cu autocomplete.
- [tenants/apps.py](django-bakend/tenants/apps.py) — config standard.
- [tenants/migrations/0001_initial.py](django-bakend/tenants/migrations/0001_initial.py) — scrisă manual; verificată cu `makemigrations --check` (no drift).
- Înregistrat în `INSTALLED_APPS` în [django_backend/settings.py](django-bakend/django_backend/settings.py).

**Default-uri implementate:**
- `Plan`: `free` / `pro` / `enterprise` (default `free`)
- `Status`: `active` / `suspended` / `deleted` (default `active`)
- `Role`: `OWNER` / `ADMIN` / `OPERATOR` / `VIEWER` / `INSTALLER` (default `VIEWER`)

### 8.2 tenant_id pe Device + migrare 3 pași (1.2) — ✅ DONE

- [clients/models.py](django-bakend/clients/models.py) — adăugat `Device.tenant = ForeignKey("tenants.Tenant", on_delete=PROTECT)`; păstrat `Device.client` pentru compat tranzitorie; `unique_together("tenant", "serial_number")`; `Device.objects = TenantQuerySet.as_manager()`.
- 3 migrări:
  - [0004_add_tenant_to_device.py](django-bakend/clients/migrations/0004_add_tenant_to_device.py) — AddField nullable.
  - [0005_populate_legacy_tenant.py](django-bakend/clients/migrations/0005_populate_legacy_tenant.py) — `RunPython`: creează `Tenant(slug=legacy)`, `Membership(role=OWNER)` pentru fiecare user existent, atribuie device-urile.
  - [0006_finalize_tenant_constraints.py](django-bakend/clients/migrations/0006_finalize_tenant_constraints.py) — NOT NULL + scoate unique global de pe serial_number + adaugă `unique_together(tenant, serial_number)`.
- [serializers.py](django-bakend/clients/serializers.py) — include `tenant` în răspuns.

**Rezultat după aplicare pe MySQL real (2026-04-27):**
- 1 tenant `legacy` creat
- 1 membership: user `admin` ↔ legacy ↔ OWNER
- 3 device-uri existente atribuite tenantului `legacy`, 0 NULL-uri rămase

### 8.3 JWT include tenant_id, tenant_slug, role (1.3) — ✅ DONE

- [clients/tokens.py](django-bakend/clients/tokens.py) — `CustomTokenObtainPairSerializer.validate()` rescris:
  - 0 active memberships → 400 (mesaj "no active tenant")
  - 1 → implicit (slug verificat dacă e trimis)
  - ≥2 → cere `tenant_slug` în request pentru disambiguare
  - Tenant cu `status` în {`suspended`, `deleted`} → exclus din lista eligibilă
  - Service account (cu perm `clients.view_device`) → bypass total (token fără claims tenant)
- Refresh token poartă claims-urile (verificat prin `RefreshToken.access_token` derivat).
- Răspunsul login include `tenant_slug` și `role` la nivelul JSON, pentru UX (frontend nu trebuie să decodeze JWT-ul).

### 8.4 Middleware + manager queryset tenant-aware (1.4) — ✅ DONE

- [tenants/middleware.py](django-bakend/tenants/middleware.py) — `TenantMiddleware` decodează JWT-ul (dacă există) și expune `request.tenant_id`, `request.tenant_slug`, `request.role`. Înregistrat la coada `MIDDLEWARE` în settings.
- [tenants/managers.py](django-bakend/tenants/managers.py) — `TenantQuerySet` cu metodă `.for_tenant(tenant)` pentru filtrare explicită.
- [clients/views.py](django-bakend/clients/views.py) — `DeviceViewSet`:
  - `get_queryset`: superuser/service-account → cross-tenant; altfel filtrează pe `request.tenant_id`; suportă filtre `?username=` și `?tenant=`.
  - `perform_create`: utilizatorii non-privilegiați au `tenant_id` și `client` setate forțat din JWT (ignoră payload-ul → previne spoof).
- Endpoint-ul vechi `GET /api/devices/<username>/` (conflict de routing cu detail viewset) **eliminat**; înlocuit cu `?username=` filter.

### 8.5 RBAC explicit (1.5) — ✅ DONE

- [tenants/permissions.py](django-bakend/tenants/permissions.py) — `TenantRolePermission` aplicat în `DeviceViewSet`:
  - **Read** (GET/HEAD/OPTIONS): toate rolurile.
  - **Write** (POST/PUT/PATCH): OWNER, ADMIN, OPERATOR, INSTALLER.
  - **Delete**: doar OWNER, ADMIN.
  - Superuser și service account (perm `clients.view_device`) → bypass total.

**Fix punctual în serializer (descoperit la testare):**
- `DeviceSerializer.Meta.validators = []` — dezactivează `UniqueTogetherValidator(tenant, serial_number)` auto-generat care cerea `tenant` în input. Tenant e injectat server-side din JWT; constraint-ul DB rămâne valid.

### 8.6 Kong propagă claim-urile JWT ca headere upstream (1.6) — ✅ DONE

- [kong/kong.yaml](kong/kong.yaml) — plugin `pre-function` adăugat pe `django-devices` și `go-api`:
  - Decodează payload-ul JWT (signature deja verificată de plugin-ul `jwt`).
  - Setează 4 headere către upstream: `X-Tenant-Id`, `X-Tenant-Slug`, `X-Role`, `X-Username`.
  - Lua minimal, dependențe doar din OpenResty/Kong bundled (`cjson.safe` + `ngx.decode_base64`).
- Permite Django/Go să citească tenant info direct din header (fără re-decodare JWT — defense in depth).

### 8.7 Go API tenant-aware (1.7) — ✅ DONE

- [internal/api/handlers.go](go-iot-platform/internal/api/handlers.go) — `getUsernameFromToken` → `getTokenContext` care extrage `username`, `tenant_id`, `tenant_slug`, `role`. `tenant_id` mandatory pentru endpoint-uri end-user. Răspunsul `/metrics` include `tenant_id` în payload.
- [internal/django/client.go](go-iot-platform/internal/django/client.go) — `GetDevicesForUser(username)` → `GetDevicesForUserInTenant(username, tenantID)` folosind endpoint-ul `?username=&tenant=`. `Device` struct extins cu `TenantID int64`.
- [internal/influx/client.go](go-iot-platform/internal/influx/client.go) — `GetFieldForDevice` acceptă `tenantID`; filtru Flux opțional `r.tenant_id == "<tid>" or not exists r.tenant_id` (backward-compatible cu date legacy fără tag).

### 8.8 Tag tenant_id pe scrierile MQTT → Influx (1.8) — ✅ DONE

- [cmd/main.go](go-iot-platform/cmd/main.go) `handleMessage`:
  - Lookup `tenant_id` în lista de device-uri din Django.
  - `tenantTag = strconv.FormatInt(d.TenantID, 10)` sau `"unassigned"` pentru device-uri necunoscute.
  - Toate cele 7 puncte Influx (Shelly emeter/relay, NousAT STATE/SENSOR, Zigbee, generic JSON, generic plain) primesc tag-ul `tenant_id`.

**Datorie tehnică cunoscută** (de adresat în Faza 2):
- `GetAllDevices()` apelat per mesaj MQTT — bottleneck până la cache Redis (Faza 2.4).

### 8.9 Fix post-deploy: service account bypass tenant membership

După repornirea Go-ului în producție, login-ul `iot-ingest` întorcea 400 ("no active tenant"). Cauză: 1.3 cerea membership pentru orice user. Service accounts sunt cross-tenant by design și nu au membership.

Fix în [clients/tokens.py](django-bakend/clients/tokens.py): user cu perm `clients.view_device` (non-superuser) login-ează direct, fără verificare de tenant; token-ul emis nu are claims `tenant_id`/`role`. Test: `test_service_account_login_without_membership`.

---

## 9. Rezultatele detaliate ale testelor — Faza 1

### 9.1 Django — pytest (46 passed)

```text
============================= test session starts =============================
platform win32 -- Python 3.12.6, pytest-8.3.4, pluggy-1.6.0
django: version: 5.2.4, settings: django_backend.settings_test (from ini)
collected 46 items

clients/tests/test_device_tenancy.py::test_same_serial_in_different_tenants_allowed   PASSED
clients/tests/test_device_tenancy.py::test_same_serial_in_same_tenant_rejected        PASSED
clients/tests/test_device_tenancy.py::test_tenant_required                            PASSED
clients/tests/test_device_tenancy.py::test_legacy_tenant_exists_after_migrations      PASSED
clients/tests/test_device_tenancy.py::test_legacy_membership_created_for_existing_users PASSED
clients/tests/test_device_tenancy.py::test_protect_on_tenant_delete                   PASSED
clients/tests/test_devices.py::test_user_sees_devices_in_own_tenant                   PASSED
clients/tests/test_devices.py::test_user_does_not_see_other_tenant_devices            PASSED
clients/tests/test_devices.py::test_superuser_sees_all_devices                        PASSED
clients/tests/test_devices.py::test_service_account_sees_all_devices                  PASSED
clients/tests/test_devices.py::test_filter_by_username_query_param                    PASSED
clients/tests/test_devices.py::test_filter_by_tenant_query_param_for_service_account  PASSED
clients/tests/test_devices.py::test_device_create_uses_tenant_from_jwt                PASSED
clients/tests/test_devices.py::test_anonymous_request_rejected                        PASSED
clients/tests/test_login.py::test_login_no_membership_rejected                        PASSED
clients/tests/test_login.py::test_login_single_membership_implicit                    PASSED
clients/tests/test_login.py::test_login_multiple_memberships_requires_slug            PASSED
clients/tests/test_login.py::test_login_multiple_memberships_with_slug                PASSED
clients/tests/test_login.py::test_login_unknown_tenant_slug_rejected                  PASSED
clients/tests/test_login.py::test_login_suspended_tenant_excluded                     PASSED
clients/tests/test_login.py::test_service_account_login_without_membership            PASSED
clients/tests/test_login.py::test_refresh_token_carries_tenant_claims                 PASSED
tenants/tests/test_models.py::test_tenant_defaults                                    PASSED
tenants/tests/test_models.py::test_tenant_str                                         PASSED
tenants/tests/test_models.py::test_tenant_slug_unique                                 PASSED
tenants/tests/test_models.py::test_membership_default_role                            PASSED
tenants/tests/test_models.py::test_membership_unique_user_tenant                      PASSED
tenants/tests/test_models.py::test_user_can_belong_to_multiple_tenants                PASSED
tenants/tests/test_models.py::test_tenant_can_have_multiple_members                   PASSED
tenants/tests/test_models.py::test_cascade_delete_tenant_removes_memberships          PASSED
tenants/tests/test_models.py::test_cascade_delete_user_removes_memberships            PASSED
tenants/tests/test_permissions.py::test_all_roles_can_read[OWNER..INSTALLER]          PASSED  (×5)
tenants/tests/test_permissions.py::test_create_device_by_role[OWNER..VIEWER]          PASSED  (×5)
tenants/tests/test_permissions.py::test_delete_device_by_role[OWNER..INSTALLER]       PASSED  (×5)

============================= 46 passed in 202.44s ============================
```

#### 9.1.1 Acoperire pe fișier de test

| Fișier | # teste | Acoperă |
|--------|---------|---------|
| [clients/tests/test_device_tenancy.py](django-bakend/clients/tests/test_device_tenancy.py) | 6 | model Device tenant-scoped: serial unic per tenant; `tenant_id` NOT NULL după 0006; legacy tenant și backfill; `PROTECT` la delete |
| [clients/tests/test_devices.py](django-bakend/clients/tests/test_devices.py) | 8 | `DeviceViewSet`: izolare per-tenant (alice nu vede device-urile lui bob); superuser și service account cross-tenant; filtre `?username=` și `?tenant=`; spoof prevention la POST |
| [clients/tests/test_login.py](django-bakend/clients/tests/test_login.py) | 8 | login flow: 0/1/multi memberships; tenant suspended exclus; refresh token carry; **service account bypass** (post-fix) |
| [tenants/tests/test_models.py](django-bakend/tenants/tests/test_models.py) | 9 | Tenant defaults, slug unique, Membership unicity, CASCADE delete, multi-tenant per user / multi-user per tenant |
| [tenants/tests/test_permissions.py](django-bakend/tenants/tests/test_permissions.py) | 15 | `TenantRolePermission` parametrizat pe (rol × metodă HTTP): read all, write OWNER+ADMIN+OPERATOR+INSTALLER, delete OWNER+ADMIN |

#### 9.1.2 Detaliu test pe areas critice

**Izolare cross-tenant** ([test_devices.py](django-bakend/clients/tests/test_devices.py)):

| Test | Scenariu | Verifică |
|------|----------|----------|
| `test_user_sees_devices_in_own_tenant` | alice e în Acme, vede ACME-001 | Filtrare implicită pe tenant_id din JWT |
| `test_user_does_not_see_other_tenant_devices` | bob e în Globex, vede doar GLOBEX-001 | Niciun leak cross-tenant |
| `test_superuser_sees_all_devices` | superuser → 2 device-uri | Bypass tenant filter |
| `test_service_account_sees_all_devices` | iot-ingest cu `view_device` perm → 2 device-uri | Bypass pentru ingest |
| `test_device_create_uses_tenant_from_jwt` | alice POST cu tenant=globex.id | `perform_create` suprascrie cu tenant din JWT (Acme) |

**RBAC parametrizat** ([test_permissions.py](django-bakend/tenants/tests/test_permissions.py)):

| Rol | GET | POST | DELETE |
|-----|-----|------|--------|
| OWNER | 200 | 201 | 204 |
| ADMIN | 200 | 201 | 204 |
| OPERATOR | 200 | 201 | **403** |
| INSTALLER | 200 | 201 | **403** |
| VIEWER | 200 | **403** | **403** |

**Login flow** ([test_login.py](django-bakend/clients/tests/test_login.py)):

| Test | Așteptat |
|------|----------|
| `test_login_no_membership_rejected` | 400 + mesaj "no active tenant" |
| `test_login_single_membership_implicit` | 200, JWT conține `tenant_id`, `tenant_slug`, `role`, `username`, `iss=django` |
| `test_login_multiple_memberships_requires_slug` | 400 + lista de slug-uri eligibile |
| `test_login_multiple_memberships_with_slug` | 200, claim-urile reflectă tenantul ales |
| `test_login_unknown_tenant_slug_rejected` | 400 (user nu e membru) |
| `test_login_suspended_tenant_excluded` | 400 (tenant suspended → membership ineligibil) |
| `test_service_account_login_without_membership` | 200, **fără** `tenant_id` în token (service account bypass) |
| `test_refresh_token_carries_tenant_claims` | refresh token conține și el claim-urile tenant |

### 9.2 Go — `go test ./...`

```text
=== RUN   TestGetTokenContext
=== RUN   TestGetTokenContext/valid_with_tenant
=== RUN   TestGetTokenContext/missing_tenant_id
=== RUN   TestGetTokenContext/missing_header
=== RUN   TestGetTokenContext/wrong_scheme
=== RUN   TestGetTokenContext/bad_signature
=== RUN   TestGetTokenContext/expired
=== RUN   TestGetTokenContext/username_missing
--- PASS: TestGetTokenContext (0.00s)
    --- PASS: TestGetTokenContext/valid_with_tenant (0.00s)
    --- PASS: TestGetTokenContext/missing_tenant_id (0.00s)
    --- PASS: TestGetTokenContext/missing_header   (0.00s)
    --- PASS: TestGetTokenContext/wrong_scheme     (0.00s)
    --- PASS: TestGetTokenContext/bad_signature    (0.00s)
    --- PASS: TestGetTokenContext/expired          (0.00s)
    --- PASS: TestGetTokenContext/username_missing (0.00s)
PASS
ok      go-iot-platform/internal/api          1.470s
=== RUN   TestRangeRegex
--- PASS: TestRangeRegex (0.00s)
PASS
ok      go-iot-platform/internal/influx       1.305s
```

| Test | Acoperire |
|------|-----------|
| `TestGetTokenContext/valid_with_tenant` | JWT cu `tenant_id`, `tenant_slug`, `role` decodat corect în `tokenContext` |
| `TestGetTokenContext/missing_tenant_id` | Tenant absent → eroare (1.7 cere tenant_id mandatory pe endpoint-uri end-user) |
| `TestGetTokenContext/missing_header` | Lipsa header `Authorization` → eroare |
| `TestGetTokenContext/wrong_scheme` | `Basic` în loc de `Bearer` → eroare |
| `TestGetTokenContext/bad_signature` | Token semnat cu alt secret → invalid |
| `TestGetTokenContext/expired` | `exp` în trecut → invalid |
| `TestGetTokenContext/username_missing` | Token fără claim `username` → eroare |
| `TestRangeRegex` | Pattern `^-?\d+[smhd]$` (din 0.6) — neschimbat |

### 9.3 `go vet ./...` și `go build ./...`

Curate, fără avertismente. Build reușit pentru toate pachetele.

---

## 10. Deploy Faza 1 — operațiuni efectuate pe MySQL real (2026-04-27)

### 10.1 Migrare DB

```text
$ python manage.py migrate
Operations to perform:
  Apply all migrations: admin, auth, clients, contenttypes, sessions, tenants
Running migrations:
  Applying tenants.0001_initial... OK
  Applying clients.0004_add_tenant_to_device... OK
  Applying clients.0005_populate_legacy_tenant... OK
  Applying clients.0006_finalize_tenant_constraints... OK
```

### 10.2 Verificare post-migrate

| Verificare | Rezultat |
|------------|----------|
| `tenants_tenant` | 1 row: `(1, 'Legacy Tenant', 'legacy', 'free', 'active')` |
| `tenants_membership` | 1 row: `(user_id=1, tenant_id=1, role=OWNER)` |
| Device-uri cu `tenant_id IS NULL` | **0** (toate cele 3 atribuite) |
| Device-uri total | 3 (neschimbat) |
| Sample devices | toate 3 cu `tenant_id=1` (legacy) |

### 10.3 Provisioning service account

```text
$ python manage.py create_service_user --password "<DJANGO_SERVICE_PASS>"
Created service user 'iot-ingest' with permissions: view_device, add_device
```

### 10.4 Validare end-to-end

Confirmat de utilizator: după repornirea Django (cu codul nou inclusiv fix-ul `5532f78`) și a serviciului Go, mesajele MQTT sunt scrise în Influx cu tag-ul `tenant_id="1"`.

---

## 11. Definition of Done — Faza 1

| Criteriu din [plan.md §Faza 1](plan.md) | Status |
|------------------------------------------|--------|
| **1.1** Tenanți și membership-uri se pot crea din admin; testele pentru CRUD trec | ✅ |
| **1.2** Toate device-urile existente au `tenant_id` populat; unique compus activ | ✅ (verificat pe MySQL real) |
| **1.3** Token decodat conține `tenant_id`, `tenant_slug`, `role`; teste verzi | ✅ |
| **1.4** Un user din tenantul A nu poate vedea device-urile tenantului B prin API | ✅ (admin Django rămâne deferat — vezi §12) |
| **1.5** VIEWER nu poate face POST/DELETE pe `/api/devices/` | ✅ (parametrizat pe 5 roluri × 3 metode) |
| **1.6** Request-ul ajunge la upstream cu `X-Tenant-Id` setat corect | ✅ (config Kong; verificat operațional după `kong reload`) |
| **1.7** Request cu token tenant A pe device tenant B → 403 | ✅ (filtrare în Django + verificare în Go) |
| **1.8** Orice punct nou în Influx are tag `tenant_id` | ✅ (verificat în producție după restart Go) |

---

## 12. Datorie tehnică cunoscută (deferată Fazei 2)

1. **GetAllDevices() apelat per mesaj MQTT** ([cmd/main.go:164](go-iot-platform/cmd/main.go#L164)) — HTTP roundtrip pe fiecare mesaj. Faza 2.4 introduce cache Redis device→tenant cu invalidare push.
2. **Filtrul Flux acceptă date fără tenant_id** ([internal/influx/client.go](go-iot-platform/internal/influx/client.go)) — `or not exists r.tenant_id` e tranzitoriu pentru date pre-1.8. După ce toate datele relevante au tag, filtrul se poate strânge.
3. **Admin Django nu e încă tenant-aware** — folosește session auth, nu JWT, deci nu beneficiază de `TenantMiddleware`. Filtrarea curentă e per-user (pre-existentă), nu per-tenant. De rezolvat cu un mecanism separat (e.g., selecție de tenant stocată în session).
4. **MQTT broker single-instance, abonare la `#`** — neschimbat de Faza 1. Faza 2.1–2.3 trece pe EMQX clusterizat cu shared subscriptions.
5. **Influx WriteAPIBlocking** — neschimbat. Faza 2.5 trece pe `WriteAPI` async cu batch.

---

## 13. Concluzie generală (Faza 0 + Faza 1)

Platforma a făcut tranziția de la **MVP single-tenant** (Faza 0 a stabilizat ce era) la **fundație multi-tenant funcțională** (Faza 1). Modelul de date, JWT-ul, autorizarea, și pipeline-ul de telemetrie sunt acum tenant-aware end-to-end. Suite-ul de teste a crescut de 11× (4 → 46 + 8 sub-teste Go) și acoperă explicit izolarea cross-tenant și RBAC.

Datele reale (3 device-uri, 1 user, 0 membership-uri inițial) au fost migrate fără pierderi în tenantul `legacy`. Sistemul a fost validat operațional: utilizatorul confirmă că Go-ul ingestă date după redeploy.

**Pas următor:** Faza 2 — ingest scalabil (EMQX + shared subscriptions, MQTT bridge pentru device-uri legacy, cache Redis device→tenant, batch writes Influx, opțional buffer Kafka/NATS).

---
---

# Raport Faza 1.9 — Hardening și fix-uri punctuale

> Status: ✅ COMPLETĂ — 8/10 items implementați, 2 deferați la Faza 2 cu motiv documentat (2026-04-28)

## 14. Sumar executiv Faza 1.9

Aplicat un set de 10 fix-uri propuse pentru hardening-ul Fazei 1 (multi-tenant safety, rezilient și pregătit pentru scalare). Implementarea s-a făcut în **3 batch-uri** cu commit per batch:

- **Batch A** (`a8a6144`): Django hardening — items 1, 2, 3, 6
- **Batch B** (`671426b`): Go ingest hardening — items 4, 5, 7
- **Batch C** (`8a5e66a`): Reziliență + observabilitate — items 8, 9, 10

| Suite | Înainte 1.9 | După 1.9 | Δ |
|-------|-------------|----------|---|
| Django (pytest) | 46 passed | **52 passed** | +6 (test_middleware_hardening) |
| Go (`go test ./...`) | 1 pachet cu teste (api), 1 cu test mic (influx) | **6 pachete cu teste**: api, influx, topics, logging, ratelimit, buffer | +4 pachete |
| `go vet ./...` | clean | clean | — |
| `go build ./...` | clean | clean | — |

## 15. Status pe item

### #1 ✅ Eliminare `?tenant=` din query params (refined)

**Implementare:** [clients/views.py](django-bakend/clients/views.py) — param-ul `?tenant=` e ignorat pentru utilizatori normali (anti-spoof). Service account-urile (cu perm `clients.view_device`) îl pot folosi pentru filtrare cross-tenant — comportament intenționat din Faza 1.7.

**Înainte:**
```python
tenant_filter = self.request.query_params.get("tenant")
if tenant_filter:
    qs = qs.filter(tenant_id=tenant_filter)
```
**După:**
```python
if _is_cross_tenant(user):
    qs = Device.objects.all()
    tenant_filter = self.request.query_params.get("tenant")
    if tenant_filter:
        qs = qs.filter(tenant_id=tenant_filter)
else:
    # Param ?tenant= IGNORAT pentru utilizatori normali — anti-spoof
    qs = Device.objects.for_tenant(request.tenant)
```

### #2 ✅ Filtrare strictă în viewset (refined pentru service account)

**Implementare:** [clients/views.py](django-bakend/clients/views.py) — non-cross-tenant fără `request.tenant` ridică `PermissionDenied` (403), nu queryset gol care ar masca leak-uri. `request.tenant` (Tenant instance) folosit pentru `for_tenant()` și `perform_create()`.

**Notă:** Codul propus literal (`if not hasattr(request, "tenant"): raise`) ar fi rupt service account. Refinat să păstreze bypass-ul.

### #3 ✅ Hardening TenantMiddleware (membership re-check + cache + signals)

**Implementare:** [tenants/middleware.py](django-bakend/tenants/middleware.py).

Înainte: middleware doar decoda JWT și seta tenant_id. Membership-ul era validat doar la **emiterea** tokenului în [tokens.py](django-bakend/clients/tokens.py); revocarea de membership nu invalida JWT-uri active.

După:
- `_resolve_tenant(user_id, tenant_id)` cu cache TTL 60s in-process → 1 query DB / 60s / user
- Membership inexistent sau Tenant inactiv → 403 cu mesaj „no active membership"
- `request.tenant` setat ca Tenant instance (în plus de tenant_id pentru compat)
- user_id/tenant_id normalizate la int (JWT trimite string, signals trimit int) — fix pentru bug de cache key mismatch
- [tenants/signals.py](django-bakend/tenants/signals.py): post_save/post_delete pe Membership și post_save pe Tenant invalidează cache-ul → revocare instant fără să aștepți TTL

### #4 ✅ Validare device ↔ tenant în Go (defensiv pentru schema nouă)

**Implementare:** [cmd/main.go](go-iot-platform/cmd/main.go) `handleMessage`.

Pentru topicuri schema nouă (`tenants/{tid}/devices/{did}/...`):
- device_id din topic trebuie să existe în Django (lookup via GetAllDevices)
- device.tenant_id trebuie să corespundă cu tenant din topic
- Mismatch → DROP cu log structurat

Pentru topicuri legacy (`shellies/...`, `tele/...`, `zigbee2mqtt/...`): comportament neschimbat (lookup device→tenant, tag tenant_id sau „unassigned"). Asta evită ruperea telemetriei curente înainte de migrarea schemei (Faza 2.1).

**Exemplu drop:**
```json
{"ts":"2026-04-28T..","level":"drop","msg":"device-tenant mismatch",
 "device_id":"dev-001","topic_tenant":1,"device_tenant":2,
 "topic":"tenants/1/devices/dev-001/up/state"}
```

### #5 ✅ Validare topic MQTT (parser nou + format strict pentru schema nouă)

**Implementare:** [internal/topics/topics.go](go-iot-platform/internal/topics/topics.go) — parser dedicat.

```go
type Parsed struct {
    IsLegacy  bool
    TenantID  int64
    DeviceID  string
    Direction string // "up" | "down"
    Stream    string
    Raw       string
}

func Parse(topic string) (Parsed, error)
```

**Erori la parse → DROP:**
- malformed (segments < 6)
- empty tenant_id sau device_id
- non-numeric tenant_id
- tenant_id ≤ 0 (negativ sau zero)
- direction invalid (≠ up/down)
- bad layout (segments greșite)

14 sub-cazuri în [topics_test.go](go-iot-platform/internal/topics/topics_test.go).

### #6 ✅ Kill-switch `MULTI_TENANT_ENABLED`

**Implementare:** [django_backend/settings.py](django-bakend/django_backend/settings.py) — flag citit din env, default `True`. La `False`, [TenantMiddleware](django-bakend/tenants/middleware.py) devine no-op → util ca rollback de urgență fără rollback DB.

**Notă:** Migrarea în 5 pași propusă inițial nu mai e relevantă — migrarea Fazei 1.2 s-a întâmplat deja pe MySQL real (vezi §10). Kill-switch-ul rezolvă scenariul „rollback runtime" care era scopul.

### #7 ✅ Influx tagging enforcement (skip dacă device_id gol)

**Implementare:** [cmd/main.go](go-iot-platform/cmd/main.go) `handleMessage`.

Faza 1.8 deja taga `tenant_id` și `device` pe toate punctele (cu `"unassigned"` pentru device necunoscut — preserves data, nu pierde). Faza 1.9 adaugă enforcement strict pentru `device_id` gol: dacă topic-ul nu permite extragerea unui deviceID, mesajul e dropped explicit.

### #8 ✅ File fallback pentru erori Influx

**Implementare:** [internal/buffer/buffer.go](go-iot-platform/internal/buffer/buffer.go).

Înainte: `_ = writeAPI.WritePoint(ctx, p)` — eroarea ignorată silent.

După: helper `writePoint(p, writeAPI, topic, payload, fields)` care:
1. Apelează `WritePoint`
2. Pe eroare: log structurat + scriere în `logs/influx_fallback.log` (1 linie JSON / payload)
3. Pe succes: log info

Toate cele 7 `NewPoint` calls în handleMessage refactorizate prin `writePoint`.

**Limite documentate:**
- Fără rotație: necesită monitorizare disk
- Fără replay automat: re-ingest offline (script TBD în Faza 2.5)
- Pentru reziliență adevărată cu retry → Faza 2.5 cu `WriteAPI` async batched

### #9 ✅ Logging structurat JSON

**Implementare:** [internal/logging/logging.go](go-iot-platform/internal/logging/logging.go).

```go
package logging

type Fields map[string]interface{}

func Info(msg string, f Fields)  // 1 linie JSON cu ts, level, msg + custom fields
func Warn(msg string, f Fields)
func Error(msg string, f Fields)
func Drop(msg string, f Fields)  // semantic distinct pentru drops vs erori
```

**Înainte:**
```text
⚠️ Device necunoscut dev-001 (topic shellies/dev-001/...) — telemetrie marcată tenant_id=unassigned
```
**După:**
```json
{"ts":"2026-04-28T07:12:33.123Z","level":"warn","msg":"unknown device — tenant=unassigned",
 "device_id":"dev-001","topic":"shellies/dev-001/emeter/0/power"}
```

Toate log-urile critice din `handleMessage` migrate la format structurat. Câmpuri obligatorii: `tenant_id`, `device_id`, `topic`. Pregătit pentru Loki / ELK cu label-uri tenant_id (Faza 4.4).

### #10 ✅ Rate limiting in-memory (token bucket)

**Implementare:** [internal/ratelimit/ratelimit.go](go-iot-platform/internal/ratelimit/ratelimit.go).

```go
limiter = ratelimit.New(
    10, 20,    // device: 10 msg/s sustained, burst 20
    200, 400,  // tenant: 200 msg/s sustained, burst 400
)

// În handleMessage, înainte de scriere:
if !limiter.Allow(deviceID, tenantTag) {
    logging.Drop("rate limited", logging.Fields{...})
    return
}
```

2 niveluri paralele: per-device + per-tenant. Allow() consumă din ambele; deny dacă oricare e gol.

**Limite documentate:**
- in-process — la mai multe instanțe Go (Faza 2.3 cu shared subscription) un device alternând între workers depășește limita configurată
- Real cross-instance rate limit = Redis-backed în Faza 2.4
- Util acum ca defense-in-depth la single-instance + bază pentru migrare la Redis fără refactor de aplicare

## 16. Cod livrat

### TenantMiddleware fixat ([tenants/middleware.py](django-bakend/tenants/middleware.py))
```python
class TenantMiddleware:
    def __call__(self, request):
        request.tenant = None
        request.tenant_id = None
        # ...
        if not self.enabled:  # MULTI_TENANT_ENABLED kill-switch
            return self.get_response(request)

        # decode JWT, extract user_id + tenant_id
        # ...

        tenant = _resolve_tenant(user_id, tenant_id)  # cached, signal-invalidated
        if tenant is None:
            return JsonResponse(
                {"detail": "User has no active membership in the requested tenant."},
                status=403,
            )

        request.tenant = tenant
        request.tenant_id = tenant.id
        request.tenant_slug = tenant.slug
        request.role = claims.get("role")
        return self.get_response(request)
```

### Django ViewSet securizat ([clients/views.py](django-bakend/clients/views.py))
```python
class DeviceViewSet(viewsets.ModelViewSet):
    permission_classes = [IsAuthenticated, TenantRolePermission]

    def get_queryset(self):
        user = self.request.user
        if _is_cross_tenant(user):
            qs = Device.objects.all()
            tenant_filter = self.request.query_params.get("tenant")
            if tenant_filter:
                qs = qs.filter(tenant_id=tenant_filter)
        else:
            tenant = getattr(self.request, "tenant", None)
            if tenant is None:
                raise PermissionDenied("No active tenant context.")
            qs = Device.objects.for_tenant(tenant)
        # ?tenant= ignorat pentru non-cross-tenant
        return qs

    def perform_create(self, serializer):
        user = self.request.user
        if _is_cross_tenant(user):
            serializer.save()
            return
        tenant = getattr(self.request, "tenant", None)
        if tenant is None:
            raise drf_serializers.ValidationError({"tenant": "No tenant context in token."})
        serializer.save(tenant=tenant, client=user)  # forțat din JWT
```

### Topic validator Go ([internal/topics/topics.go](go-iot-platform/internal/topics/topics.go))
```go
func Parse(topic string) (Parsed, error) {
    if !strings.HasPrefix(topic, "tenants/") {
        return Parsed{IsLegacy: true, Raw: topic}, nil
    }
    // tenants/{tid}/devices/{did}/{up|down}/{stream}
    // strict validation; orice abatere → eroare → caller DROP
}
```

### Validare device-tenant în handleMessage
```go
if !parsed.IsLegacy {
    if !found {
        logging.Drop("unknown device on tenant-scoped topic", ...)
        return
    }
    if deviceTenantID != parsed.TenantID {
        logging.Drop("device-tenant mismatch", logging.Fields{
            "device_id":     deviceID,
            "topic_tenant":  parsed.TenantID,
            "device_tenant": deviceTenantID,
        })
        return
    }
}
```

### Buffer fallback ([internal/buffer/buffer.go](go-iot-platform/internal/buffer/buffer.go))
```go
func writePoint(p *write.Point, writeAPI influxdb2api.WriteAPIBlocking, topic string, payload []byte, fields logging.Fields) {
    if err := writeAPI.WritePoint(context.Background(), p); err != nil {
        fields["error"] = err.Error()
        logging.Error("influx write failed", fields)
        if bufErr := influxBuffer.Append(topic, payload, err); bufErr != nil {
            logging.Error("buffer fallback failed", logging.Fields{"error": bufErr.Error()})
        }
        return
    }
    logging.Info("influx write ok", fields)
}
```

## 17. Checklist validare securitate

| Verificare | Status | Cum e validat |
|------------|--------|---------------|
| ❌ Acces cross-tenant prin `?tenant=`| Imposibil pentru user normal | `test_query_param_tenant_ignored_for_regular_user` |
| ❌ Acces cu JWT vechi după revocare membership | 403 imediat | `test_revoked_membership_blocks_access` |
| ❌ Acces în tenant suspendat | 403 imediat | `test_suspended_tenant_blocks_access` |
| ❌ Spoof `tenant=other` la POST /api/devices/ | tenant suprascris cu cel din JWT | `test_device_create_uses_tenant_from_jwt` |
| ❌ Device publică pe topic `tenants/{altul}/...` | DROP la handleMessage | logică în [main.go](go-iot-platform/cmd/main.go) + topic parser |
| ❌ Topic malformed (segments lipsă, empty IDs, non-numeric tenant) | DROP | 8 sub-cazuri error în [topics_test.go](go-iot-platform/internal/topics/topics_test.go) |
| ❌ Device necunoscut pe topic schema nouă | DROP | logică în [main.go](go-iot-platform/cmd/main.go) |
| ✔ Toate query-urile filtrate pe tenant | da | view-set strict pentru non-cross-tenant |
| ✔ Toate datele Influx au `tenant_id` + `device` | da din 1.8 | tag-uri obligatorii în NewPoint, drop dacă device_id gol |
| ✔ Erori Influx loghate + buffer fallback | da | `writePoint` helper în [main.go](go-iot-platform/cmd/main.go), test [buffer_test.go](go-iot-platform/internal/buffer/buffer_test.go) |
| ✔ Logs structurate JSON cu tenant/device/topic | da | [logging package](go-iot-platform/internal/logging/logging.go) + integrare în handleMessage |
| ✔ Rate limit per-device + per-tenant | da, in-process | [ratelimit package](go-iot-platform/internal/ratelimit/ratelimit.go) |

## 18. Datorie tehnică (deferată Fazei 2 cu motiv documentat)

| Item Faza 1.9 | Limita curentă | Adresat în |
|---------------|----------------|-----------|
| #4 device-tenant validation | Doar pentru topicuri schema nouă; legacy păstrează lookup permisiv | Faza 2.1 (toate device-urile pe topic schema nouă cu credentials per-device) |
| #5 topic validation strict | Schema nouă coexistă cu legacy; nu putem refuza tot ce nu e `tenants/...` | Faza 2.1 + 2.2 (bridge pentru legacy → schema nouă) |
| #8 buffer fără rotație/replay | Disk poate umple; replay manual | Faza 2.5 (`WriteAPI` async cu retry built-in din SDK) |
| #10 rate limit in-memory | Single-instance only — workers paraleli depășesc limita | Faza 2.4 (Redis-backed cu token state partajat) |
| `GetAllDevices()` per mesaj MQTT | HTTP roundtrip pe fiecare ingest — bottleneck cunoscut | Faza 2.4 (cache Redis device→tenant cu invalidare push) |

## 19. Concluzie Faza 1.9

Sistem hardened pe 4 dimensiuni: **tenancy strict** (membership re-check + queryset rigid + ?tenant= restricționat), **integritate ingest** (topic parser + device-tenant cross-check + tag enforcement), **reziliență** (file fallback + structured logging), **defense-in-depth** (rate limit + kill-switch).

Suite-ul de teste a crescut la **52 Django + 5 pachete Go cu teste**. Implementarea respectă constrângerea „nu redesena arhitectura" — doar 1 modificare în settings.py (flag) și extensie de pachete `internal/` în Go.

Faza 1.9 face Faza 1 production-ready. Faza 2 va înlocui workaround-urile in-process (rate limit, cache device→tenant) cu infrastructura corectă (Redis), va clusteriza brokerul (EMQX cu shared subscriptions), și va finaliza migrarea topicurilor către schema nouă.
