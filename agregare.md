# Agregare date energetice — daily / monthly / yearly

> Analiză + propunere de soluție pentru consum casă, import/export rețea, charge/discharge baterie.

---

## 1. Context proiect (relevant pentru decizie)

| Aspect | Stare actuală | Implicație |
|---|---|---|
| **Backend Django** | Django 5.2, DRF, JWT, drf-spectacular | Adăugare app + model + endpoint trivial |
| **Database** | **MySQL** (`mysqlclient==2.2.7`) | TimescaleDB OUT (e Postgres-only). MySQL e suficient pentru row-counts ce ne așteaptă. |
| **Time zone** | `TIME_ZONE = "UTC"`, `USE_TZ = True` | Trebuie tratat explicit "ziua locală" tenant. Default UTC = day boundary la 02:00 / 03:00 RO. |
| **Time-series** | InfluxDB (3 buckets: `iot-free` / `iot-pro` / `iot-enterprise`) | Sursa raw rămâne acolo. Aggregates merg într-un al doilea store. |
| **Cron / scheduler** | **Nu există Celery** sau alt scheduler Python rulând. Redis e folosit doar pt cache + queues. Există servicii Go persistente (`mqtt-bridge`, `downlink-worker`, `rule-engine`, `main`). | Decizie cheie: **adăugăm un job runner**. |
| **Multi-tenant** | Strict isolation (`tenant_id` în Influx + `for_tenant()` în Django) | Aggregates trebuie tenant-scoped per device |
| **Apps existente** | `clients`, `tenants`, `provisioning`, `ota`, `audit`, `api_keys`, `rules`, `notifications` | Adăugăm `energy/` ca app nou |
| **Câmpuri Huawei deja parsate** | În `cmd/main.go` payload-ul SUN2000 → fields la Influx: `daily_energy_yield`, `grid_energy_imported`, `grid_energy_exported`, `battery_total_charge`, `battery_total_discharge`, `battery_day_charge_capacity`, `battery_day_discharge_capacity`, `house_load_kw_est`, `pv_input_power`, `battery_power`, `grid_power` | Schema field-urilor e fixă, putem mapa direct |
| **Tenant plans** | Free / Pro / Enterprise → bucket Influx separat | Aggregator-ul trebuie să știe ce bucket să citească per tenant |

---

## 2. Două tipuri de date — strategii diferite

| Tip | Field-uri | Ce facem |
|---|---|---|
| **A. Contoare cumulative** (lifetime, monoton crescătoare în kWh) | `grid_energy_imported`, `grid_energy_exported`, `accumulated_energy_yield`, `battery_total_charge`, `battery_total_discharge` | `delta = value[end] − value[start]` |
| **B. Putere instantanee** (W / kW, fluctuant) | `house_load_kw_est`, `pv_input_power`, `battery_power`, `grid_power`, `active_power` | Integrare: `kWh = mean(power_kw) × hours` SAU folosim Flux `integral()` |
| **C. Daily counters resetate de device** | `daily_energy_yield`, `battery_day_charge_capacity`, `battery_day_discharge_capacity`, `peak_active_power_day` | Citește last() la 23:59 local — e direct kWh-ul zilei |

**Concluzia tehnică:** un job care la sfârșitul zilei scrie un row `EnergyDaily` per (device, field, date) cu valoarea agregată corect, indiferent de tipul A/B/C.

---

## 3. Opțiuni examinate

### Opțiunea 1: On-the-fly în Flux la fiecare query
- ❌ Time-zone hell: `range=-24h` ≠ "azi 00:00 ora RO"
- ❌ Anual = 365 puncte first/last la fiecare refresh — costisitor
- ❌ Sensibil la device offline / Influx retention
- ✅ Zero schema nouă

**Verdict:** OK doar pentru ranges scurte ad-hoc. **NU pentru daily/monthly/yearly stabil.**

### Opțiunea 2: InfluxDB Tasks (Flux server-side)
- ✅ Native time-series
- ❌ Per-tenant config explosion (zeci de tenants × N device-uri × M fields = sute de tasks Flux)
- ❌ Time zone tot trebuie gestionat în Flux
- ❌ Debugging Flux fără stack trace
- ❌ Decuplat de Django (nu se vede în admin / nu se backfilează ușor)

**Verdict:** scalează tehnic, dar adaugă layer ops greu de menținut.

### Opțiunea 3: Pre-computed în MySQL (recomandare)
- ✅ Schema simplă (un singur model `EnergyDaily`)
- ✅ Integrate cu Django admin pentru debug / corecții manuale
- ✅ Time zone gestionat curat în Python (`pytz` / `zoneinfo`)
- ✅ Idempotent + backfill simplu prin management command
- ✅ Yearly chart = 365 rows max — query SQL trivial
- ⚠️ Necesită un job runner (vezi §6)

**Verdict:** **CÂȘTIGĂTOR** — best fit pentru stack-ul actual.

---

## 4. Schema MySQL propusă

App nou: **`energy/`** (paralel cu `rules/`, `notifications/`).

### `energy/models.py`

```python
from django.db import models


class EnergyDaily(models.Model):
    """Agregat zilnic per (device, field). O zi locală tenant = un row per field.

    `date` e ziua locală a tenantului (Europe/Bucharest pt clienții RO),
    nu UTC. Computed la 00:15 local prin job (vezi §6).

    Idempotent: re-rularea aceleiași zile face UPDATE, nu duplică.
    """

    class Source(models.TextChoices):
        DELTA = "delta", "Counter delta (last - first)"
        DAILY_LAST = "daily_last", "Daily counter, last value"
        INTEGRAL = "integral", "Power integrated over time"
        MANUAL = "manual", "Manual override / corecție"

    tenant      = models.ForeignKey("tenants.Tenant", on_delete=models.CASCADE,
                                    related_name="energy_daily")
    device      = models.ForeignKey("clients.Device", on_delete=models.CASCADE,
                                    related_name="energy_daily")
    date        = models.DateField(help_text="Ziua locală tenant (nu UTC)")
    field       = models.CharField(max_length=64,
                                   help_text="Numele câmpului din InfluxDB (ex: grid_energy_imported)")
    value       = models.FloatField(help_text="kWh sau unitatea câmpului")
    source      = models.CharField(max_length=16, choices=Source.choices)
    points      = models.PositiveIntegerField(default=0,
                                              help_text="Câte puncte raw a folosit calculul (anti-bias dacă device offline)")
    computed_at = models.DateTimeField(auto_now=True)

    class Meta:
        unique_together = ("device", "field", "date")
        indexes = [
            models.Index(fields=["tenant", "date"]),
            models.Index(fields=["device", "field", "date"]),
        ]
        ordering = ["-date"]

    def __str__(self):
        return f"{self.device.serial_number} {self.field}={self.value:.2f} on {self.date}"
```

### Migrare

```bash
python manage.py makemigrations energy
python manage.py migrate
```

### Volum estimat

| Scale | Rows/zi | Rows/an |
|---|---|---|
| 1 device × 8 fields | 8 | 2,920 |
| 100 device-uri × 8 fields | 800 | 292,000 |
| 10,000 device-uri × 8 fields | 80,000 | 29,200,000 |

Cu indici pe `(device, field, date)`, MySQL servește orice query sub 50ms până la zeci de milioane de rows. **Nu e problemă de scale pentru următorii 2-3 ani.**

---

## 5. Câmpuri de agregat — config în cod

```python
# energy/aggregation_config.py

AGGREGATIONS = [
    # Type A: lifetime counters → delta
    {"field": "grid_energy_imported",  "method": "delta",      "unit": "kWh"},
    {"field": "grid_energy_exported",  "method": "delta",      "unit": "kWh"},
    {"field": "battery_total_charge",  "method": "delta",      "unit": "kWh"},
    {"field": "battery_total_discharge","method": "delta",     "unit": "kWh"},
    {"field": "accumulated_energy_yield","method": "delta",    "unit": "kWh"},

    # Type C: daily counters resetate de device → last value at end-of-day
    {"field": "daily_energy_yield",            "method": "daily_last", "unit": "kWh"},
    {"field": "battery_day_charge_capacity",   "method": "daily_last", "unit": "kWh"},
    {"field": "battery_day_discharge_capacity","method": "daily_last", "unit": "kWh"},

    # Type B: power → integral (kWh = ∫ P dt)
    {"field": "house_load_kw_est",  "method": "integral", "unit": "kWh"},
    {"field": "pv_input_power",     "method": "integral", "unit": "kWh"},
]
```

Cu config-ul în cod, jobul iterează deterministic peste fiecare device sun2000 / shelly relevant.

---

## 6. Job runner — alegere critică (nu există Celery)

### Variantele:

| Variantă | Pro | Contra |
|---|---|---|
| **A. Celery + Celery Beat** | Standard Python, scheduler robust, retry automat, monitoring (Flower) | + 2 deps noi (celery, kombu), + worker proces, încă un layer | Crește complexitatea ops |
| **B. `manage.py rollup_daily` + system cron** | Zero deps, simplu, predictibil | Cron de configurat la deploy, fără retry built-in |
| **C. Go scheduler binar nou (`cmd/aggregator/main.go`)** | Folosește stack-ul Go existent (downlink-worker style), 1 binar mai mult, comunică direct cu Influx + apelează Django prin service token | Logica de agregare în Go, nu Python — unele aspecte (ex: backfill din admin) mai greu |

### Recomandare: **B (management command + system cron)**

**De ce B și nu A sau C:**
- Tu n-ai Celery încă; introducerea lui pentru un job/zi e overkill (Celery vine cu Redis-broker discipline, signal handling, worker scaling — nimic din astea n-ai nevoie pentru 1 task/zi)
- Logica e Python-friendly (lookup Device, compute, ORM write) — în Go ar fi reinventat ce face Django ORM
- System cron e robust dacă deploy-ul e Docker (`crond` într-un side-container) sau bare metal (`/etc/cron.d/`)
- **Backfill = simplă: `python manage.py rollup_daily --from 2026-01-01 --to 2026-05-09`**

**Dacă scale-ul crește la 10k+ device-uri** și 1 job durează > 30min, migrezi la Celery cu task-uri parallel pe device. Până atunci, supra-inginerie.

### Implementare:

#### `energy/management/commands/rollup_daily.py`

```python
"""Aggregate energy data per device, per day, in MySQL.

Usage:
    python manage.py rollup_daily              # rollup ieri (default)
    python manage.py rollup_daily --date 2026-05-08
    python manage.py rollup_daily --from 2026-01-01 --to 2026-05-09  # backfill
    python manage.py rollup_daily --device SHELF001 --date 2026-05-08  # single device
"""
import logging
from datetime import date, datetime, timedelta, time
from zoneinfo import ZoneInfo

from django.core.management.base import BaseCommand, CommandError
from django.db import transaction

from clients.models import Device
from tenants.models import Tenant
from energy.models import EnergyDaily
from energy.aggregation_config import AGGREGATIONS
from energy.influx_client import (
    query_field_delta,    # last - first în interval (UTC)
    query_field_last,     # last value în interval
    query_field_integral, # ∫ P dt în kWh
)

logger = logging.getLogger(__name__)
LOCAL_TZ = ZoneInfo("Europe/Bucharest")  # Or per-tenant: tenant.timezone


class Command(BaseCommand):
    help = "Compute daily energy aggregates and store in EnergyDaily."

    def add_arguments(self, parser):
        parser.add_argument("--date", help="ISO date (YYYY-MM-DD), local tenant TZ")
        parser.add_argument("--from", dest="from_", help="Backfill range start")
        parser.add_argument("--to", dest="to_", help="Backfill range end (inclusive)")
        parser.add_argument("--device", help="Single device serial (default: all)")
        parser.add_argument("--tenant", type=int, help="Tenant id (default: all active)")
        parser.add_argument("--dry-run", action="store_true")

    def handle(self, *args, **opts):
        # Compute list of dates
        if opts["from_"] and opts["to_"]:
            d_from = date.fromisoformat(opts["from_"])
            d_to = date.fromisoformat(opts["to_"])
            dates = [d_from + timedelta(days=i) for i in range((d_to - d_from).days + 1)]
        elif opts["date"]:
            dates = [date.fromisoformat(opts["date"])]
        else:
            dates = [date.today() - timedelta(days=1)]  # ieri

        # Filter devices
        devices = Device.objects.all().select_related("tenant")
        if opts["tenant"]:
            devices = devices.filter(tenant_id=opts["tenant"])
        if opts["device"]:
            devices = devices.filter(serial_number=opts["device"])

        for d in dates:
            for device in devices:
                self._rollup_device_date(device, d, dry_run=opts["dry_run"])

    @transaction.atomic
    def _rollup_device_date(self, device: Device, day: date, dry_run: bool):
        # Convert local day boundaries to UTC for Influx queries
        start_local = datetime.combine(day, time.min, tzinfo=LOCAL_TZ)
        end_local = start_local + timedelta(days=1)
        start_utc = start_local.astimezone(ZoneInfo("UTC"))
        end_utc = end_local.astimezone(ZoneInfo("UTC"))

        for cfg in AGGREGATIONS:
            field = cfg["field"]
            method = cfg["method"]

            try:
                if method == "delta":
                    val, points = query_field_delta(device, field, start_utc, end_utc)
                elif method == "daily_last":
                    val, points = query_field_last(device, field, start_utc, end_utc)
                elif method == "integral":
                    val, points = query_field_integral(device, field, start_utc, end_utc)
                else:
                    continue
            except Exception as exc:
                logger.warning("rollup %s/%s/%s failed: %s",
                               device.serial_number, field, day, exc)
                continue

            if val is None or points == 0:
                continue  # device offline ziua aia

            if dry_run:
                self.stdout.write(f"  [DRY] {device.serial_number} {field}={val:.3f} on {day} (n={points})")
                continue

            EnergyDaily.objects.update_or_create(
                device=device,
                field=field,
                date=day,
                defaults={
                    "tenant": device.tenant,
                    "value": val,
                    "source": method,
                    "points": points,
                },
            )

        self.stdout.write(self.style.SUCCESS(f"  ✓ {device.serial_number} for {day}"))
```

#### `energy/influx_client.py`

```python
"""Helpers pentru a interoga InfluxDB direct din Django (bypass Go API).

Folosit DOAR de jobul de aggregation (server-side, are credențiale Influx full).
Pentru read-uri user-facing, treci prin Go API ca până acum.
"""
from datetime import datetime
from influxdb_client import InfluxDBClient
from django.conf import settings

from clients.models import Device


def _client():
    return InfluxDBClient(
        url=settings.INFLUX_URL,
        token=settings.INFLUX_TOKEN,
        org=settings.INFLUX_ORG,
    )


def _bucket_for_plan(plan: str) -> str:
    return {
        "free": settings.INFLUX_BUCKET_FREE,
        "pro": settings.INFLUX_BUCKET_PRO,
        "enterprise": settings.INFLUX_BUCKET_ENTERPRISE,
    }.get(plan, settings.INFLUX_BUCKET_FREE)


def query_field_delta(device: Device, field: str,
                      start: datetime, end: datetime) -> tuple[float | None, int]:
    """Returns (last - first, point_count) pentru contoare cumulative."""
    bucket = _bucket_for_plan(device.tenant.plan)
    flux = f'''
        base = from(bucket: "{bucket}")
            |> range(start: {start.isoformat()}, stop: {end.isoformat()})
            |> filter(fn: (r) => r._measurement == "devices"
                              and r.device == "{device.serial_number}"
                              and r._field == "{field}"
                              and r.tenant_id == "{device.tenant_id}")
        base |> count() |> yield(name: "count")
        base |> first() |> yield(name: "first")
        base |> last()  |> yield(name: "last")
    '''
    with _client() as cli:
        tables = cli.query_api().query(flux)
    first_v = last_v = None
    count = 0
    for table in tables:
        for rec in table.records:
            name = rec.values.get("result")
            if name == "first":  first_v = rec.get_value()
            if name == "last":   last_v = rec.get_value()
            if name == "count":  count = int(rec.get_value() or 0)
    if first_v is None or last_v is None:
        return None, count
    return last_v - first_v, count


def query_field_last(device, field, start, end):
    """Last value în interval (pentru daily counters Huawei)."""
    bucket = _bucket_for_plan(device.tenant.plan)
    flux = f'''
        from(bucket: "{bucket}")
            |> range(start: {start.isoformat()}, stop: {end.isoformat()})
            |> filter(fn: (r) => r._measurement == "devices"
                              and r.device == "{device.serial_number}"
                              and r._field == "{field}"
                              and r.tenant_id == "{device.tenant_id}")
            |> last()
    '''
    with _client() as cli:
        tables = cli.query_api().query(flux)
    for table in tables:
        for rec in table.records:
            return float(rec.get_value()), 1
    return None, 0


def query_field_integral(device, field, start, end):
    """∫ P dt → kWh, folosind Flux integral() cu unit hour."""
    bucket = _bucket_for_plan(device.tenant.plan)
    flux = f'''
        base = from(bucket: "{bucket}")
            |> range(start: {start.isoformat()}, stop: {end.isoformat()})
            |> filter(fn: (r) => r._measurement == "devices"
                              and r.device == "{device.serial_number}"
                              and r._field == "{field}"
                              and r.tenant_id == "{device.tenant_id}")
        base |> count() |> yield(name: "count")
        base |> integral(unit: 1h) |> yield(name: "integral")
    '''
    # NOTĂ: dacă field e în W (nu kW), împarte rezultatul la 1000 după.
    with _client() as cli:
        tables = cli.query_api().query(flux)
    integ = None
    count = 0
    for table in tables:
        for rec in table.records:
            name = rec.values.get("result")
            if name == "integral": integ = float(rec.get_value())
            if name == "count":    count = int(rec.get_value() or 0)
    if integ is None:
        return None, count
    return integ, count  # caller decide unitatea
```

#### Cron entry (Linux)

```cron
# /etc/cron.d/iot-rollup
# Rulare la 00:15 Europe/Bucharest (= 22:15 UTC iarna, 21:15 UTC vara)
# Configurăm crond cu TZ=Europe/Bucharest în container

15 0 * * * django /opt/iot-platform/venv/bin/python /opt/iot-platform/django-bakend/manage.py rollup_daily >> /var/log/iot/rollup.log 2>&1
```

În Docker compose, un side-container `cron` care `crond -f` cu fișier crontab montat. Sau pe bare metal, direct în `/etc/cron.d/`.

---

## 7. API public pentru frontend

App `energy/` expune:

```
GET /api/v1/energy/daily/?device=SHELF001&field=grid_energy_imported&from=2026-05-01&to=2026-05-31
→ [
    { "date": "2026-05-01", "value": 8.42, "source": "delta", "points": 1440 },
    { "date": "2026-05-02", "value": 6.15, "source": "delta", "points": 1438 },
    ...
  ]

GET /api/v1/energy/monthly/?device=SHELF001&field=...&year=2026
→ Aggregate SQL: SUM(value) GROUP BY MONTH(date)
→ [ { "month": "2026-01", "value": 245.10 }, ... ]

GET /api/v1/energy/yearly/?device=SHELF001&field=...
→ Aggregate SQL: SUM(value) GROUP BY YEAR(date)
→ [ { "year": 2025, "value": 3120.45 }, { "year": 2026, "value": 1502.30 } ]

GET /api/v1/energy/breakdown/?device=SHELF001&period=day|month|year&date=...
→ Returnează toate câmpurile pentru o perioadă (ca să faci donut consum cu PV/Bat/Grid):
  { "from_pv": 12.4, "from_battery": 5.1, "from_grid": 3.2, ...}
```

Tenant-scoped automat prin `TenantMiddleware` + filter `EnergyDaily.objects.filter(tenant=request.tenant)`.

---

## 8. Frontend: ce câștigi

### Pe SolarPage:
- **Donut zilnic correct** — citește din `/api/v1/energy/breakdown/?period=day` (nu mai face delta din Influx la fiecare refresh)
- **Bar chart lunar** — 30 zile pe axa X, kWh pe Y, una sau mai multe serii
- **Yearly chart** — 12 luni pe X, comparison year-over-year
- **Statistici totale** — "Total exportat anul ăsta: 4,532 kWh" trivial: `SUM(value) WHERE field='grid_energy_exported' AND YEAR=2026`

### Cache:
- Daily aggregates nu se schimbă după ce ziua trece → `Cache-Control: public, max-age=86400`
- Monthly/yearly la fel pentru luni/ani închise
- Doar ziua curentă (rollup-ul ei se face mâine la 00:15) e refresh-frecvent

---

## 9. Plan de implementare incremental

| Faza | Ce livrezi | Effort | Dependinte |
|---|---|---|---|
| **0. Decizie + design review** | Validezi cu mine sau echipa schema + lista câmpurilor | 30min | — |
| **1. App + Model + Migrare** | `energy/` app, `EnergyDaily`, migrare aplicată | 1h | 0 |
| **2. Influx client helpers** | `energy/influx_client.py` cu cele 3 funcții (delta/last/integral) | 2h | 1 |
| **3. Management command** | `rollup_daily` cu CLI complet (date / from-to / device / tenant / dry-run) | 3h | 2 |
| **4. Backfill istoric** | Rulezi `rollup_daily --from <prima_zi> --to <ieri>` ca să populezi tabelul | 30min run | 3 |
| **5. Cron / scheduler** | Entry crontab + log rotation | 30min | 3 |
| **6. API endpoints** | `views.py` + `urls.py` + serializers + tenant-scoping | 2h | 1, 4 |
| **7. Frontend integration** | Donut din API nou + chart lunar/anual cu Recharts | 4h | 6 |
| **8. Monitoring** | Alert dacă rollup nu a rulat azi sau a făcut < 50% din rows așteptate | 1h | 5 |

**Total: ~14h dev (~2 zile lucru) pentru întreaga pipeline.**

Backfill istoric depinde de retention-ul Influx — dacă bucket-ul Free are retention 7d, recuperezi doar ultimele 7 zile. Pentru istoric mai vechi, ai gap-uri (acceptabil, marcat `points=0`).

---

## 10. Caveats critice

1. **Time zone** — stochezi `date` în ziua locală tenant. Pentru tenants în RO, folosești `Europe/Bucharest`. Dacă vei avea tenants în alte fusuri, adaugi `tenant.timezone` și job-ul iterează.

2. **DST** — cu `zoneinfo.ZoneInfo`, conversia local→UTC e automată. Ziua de 25 octombrie are 25h, 27 martie are 23h. Acceptabil — `delta` și `integral` nu sunt afectate de durata zilei.

3. **Idempotență** — `update_or_create` pe `(device, field, date)` permite rerun fără duplicate. Critic pentru backfill.

4. **Device offline** — dacă device n-a publicat o zi, rollup-ul scrie `points=0` și skipează (în loc să creeze un row cu 0 kWh, ce ar fi interpretat greșit ca "0 consum"). Frontend afișează "no data" pentru ziua aia, nu "0".

5. **Counter rollover / reset** — dacă invertorul resetează counter-ul lifetime (rar dar posibil după service), `delta = last - first` poate fi negativ. Adaugi `max(0, delta)` la calcul + log warning.

6. **InfluxDB retention** — rollup trebuie să ruleze înainte ca datele să iasă din bucket. La Free (7d), un job ratat 7+ zile = pierdere permanentă a celor zile. Monitoring obligatoriu.

7. **Backfill safety** — flag `--dry-run` în comandă, plus eventual `--force` pentru overwrite explicit. Default = `update_or_create` care suprascrie.

8. **Performanță rollup** — pentru 1000 device-uri × 10 fields = 10000 query-uri Influx pe zi. Fiecare query ~50-200ms → total ~10-30 minute. Pentru 10k+ device-uri trece de fereastra de 1h, atunci adaugi parallelism (Celery sau goroutines via Go aggregator).

9. **Tenant suspension / deletion** — adăugă filter `tenant.status == ACTIVE` la lista de device-uri. Tenants suspended nu mai trebuie aggregate.

10. **Audit + manual override** — `source="manual"` îți permite să editezi un row din admin Django dacă datele Influx sunt corupte (incident, sensor schimbat). Job-ul automat NU suprascrie rows cu `source="manual"` (sau le suprascrie doar cu flag `--force`).

---

## 11. Decizii încă deschise (pentru tine)

| Întrebare | Default propus | Alternativă |
|---|---|---|
| Time zone per tenant? | `Europe/Bucharest` global | Adaugi `Tenant.timezone` field |
| Retenție `EnergyDaily`? | Indefinit (e mic, ~1MB/an pe device) | Truncate > 5 ani |
| Cine rulează cron-ul? | System cron + management command | Celery beat (dacă apare nevoie de task-uri repetate la <1h) |
| Job runner host? | Container `cron` în compose, sau bare metal | Adaugi un al 4-lea binar Go `cmd/aggregator` |
| Backfill automat la inserare device nou? | Manual prin `rollup_daily --device X --from Y` | Signal `post_save` pe Device care declanșează backfill |
| Frontend: chart lib? | Recharts (deja instalat) | Echarts / D3 dacă vrei interactivitate avansată |
| Endpoint rate limit? | 100 req/min/tenant | Per-user dacă e nevoie |

---

## 12. Recomandare finală — în trei bullet-uri

1. **App nou `energy/` cu model `EnergyDaily`** (1 row per device × field × zi locală tenant). MySQL e ok pentru următorii ani.

2. **Job runner = `manage.py rollup_daily` + system cron la 00:15 local**, NU Celery (overkill), NU Flux Tasks (per-tenant explosion). Backfill simplu cu `--from / --to`.

3. **API REST simplu** care servește daily/monthly/yearly prin SQL `GROUP BY` peste `EnergyDaily`. Frontend desenează direct fără să mai apeleze Influx pentru aggregate user-facing.

**Effort total ~14h pentru livrare end-to-end.** Schema rezistentă la timezone, retention Influx, device offline, counter resets. Vizualizare zilnică/lunară/anuală instantă pe frontend.
