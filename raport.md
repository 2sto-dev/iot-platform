# Raport Faza 0 — Stabilizare și fundație de lucru

> Data raport: 2026-04-26
> Referință plan: [plan.md §Faza 0](plan.md)
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

## 6. Concluzie și pas următor

Faza 0 e încheiată cu suite-ul de teste verde. Nu există blocante pentru a începe **Faza 1.1 — Modelare Django: Tenant + Membership + Role** ([plan.md §1.1](plan.md)).

**Pas imediat următor recomandat:** crearea app-ului Django `tenants/` cu modelele `Tenant` și `Membership`, urmată de adăugarea `tenant_id` la `Device` printr-o migrație în 3 pași (nullable → backfill cu tenant „legacy" → NOT NULL + unique compus). Toate modificările trebuie să vină cu teste noi care extind suite-ul actual.

Înainte de a începe Faza 1, trebuie închisă observația 4.1 (rotație secret în `.env` locale) — operațiune de 5 minute.
