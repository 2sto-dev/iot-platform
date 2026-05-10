"""Faza 5: încarcă Device Definitions YAML pentru a popula `Device.capabilities`.

Sincron cu Go-side `internal/capabilities/inheritance.go` — orice schimbare
în vocabulary/inheritance trebuie făcută în AMBELE locuri (Go + Python).

Cache module-level: load la prima accesare, apoi dict in-memory.
"""
import logging
import os
from functools import lru_cache
from typing import Iterable

from django.conf import settings

logger = logging.getLogger(__name__)


# ── Inheritance map (mirror al Go internal/capabilities/inheritance.go) ──────
# Adaugare entry nou: ADR amendment + sync ambele locuri (Go + Python).
INHERITANCE_MAP: dict[str, list[str]] = {
    "smart_plug": ["relay", "power_meter"],
    "hybrid_inverter": ["inverter", "battery"],
    "climate_sensor": ["temperature_sensor", "humidity_sensor"],
    "ev_charger": ["relay", "power_meter"],
    "smart_meter": ["power_meter"],
    "light": ["dimmer"],
}


def resolve_capabilities(declared: Iterable[str]) -> list[str]:
    """Expandeaza capabilities declared cu cele inherited prin BFS.

    Identic algoritmic cu Go `capabilities.Resolve()`. Cycles blocate prin seen-set.

    Returns lista deduplicate, în ordinea de iterare BFS.
    """
    seen: set[str] = set()
    ordered: list[str] = []
    queue = list(declared)
    while queue:
        c = queue.pop(0)
        if c in seen:
            continue
        seen.add(c)
        ordered.append(c)
        for parent in INHERITANCE_MAP.get(c, []):
            queue.append(parent)
    return ordered


# ── YAML loader ─────────────────────────────────────────────────────────────


@lru_cache(maxsize=1)
def _load_dd_directory() -> dict[str, list[str]]:
    """Citeste configs/devices/*.yaml și construiește dict device_type → resolved capabilities.

    Cache lru_cache(1) — invalidat manual cu `_load_dd_directory.cache_clear()`
    (folosit în management command sync_capabilities pentru reload).

    Path-ul DD_DIR e configurabil via settings.DD_DIR sau env DD_DIR.
    Default: <BASE_DIR>/../configs/devices.
    """
    # PyYAML e deja instalat (transitive dep paho-mqtt? să verificăm)
    try:
        import yaml  # type: ignore
    except ImportError:
        logger.warning("PyYAML not installed — capability auto-population disabled")
        return {}

    dd_dir = os.environ.get("DD_DIR") or getattr(settings, "DD_DIR", None)
    if not dd_dir:
        # Default: relative la django-bakend → repo root → configs/devices
        base_dir = os.path.abspath(os.path.join(settings.BASE_DIR, "..", "configs", "devices"))
        dd_dir = base_dir

    if not os.path.isdir(dd_dir):
        logger.warning("DD_DIR %r does not exist; capability lookup disabled", dd_dir)
        return {}

    out: dict[str, list[str]] = {}
    for fname in os.listdir(dd_dir):
        if fname.startswith(("_", ".")):
            continue
        if not fname.endswith((".yaml", ".yml")):
            continue
        path = os.path.join(dd_dir, fname)
        try:
            with open(path, encoding="utf-8") as f:
                doc = yaml.safe_load(f)
        except Exception as exc:
            logger.warning("DD YAML %s parse failed: %s", path, exc)
            continue

        if not isinstance(doc, dict):
            continue
        dd_id = doc.get("id")
        caps = doc.get("capabilities") or []
        if not dd_id or not isinstance(caps, list):
            continue

        # Mapare DD.id → device_type Django.
        # Convenția: id-urile YAML și DEVICE_CHOICES Django coincid 1:1.
        # Ex: "huawei_sun2000_3phase" în YAML → "sun2000" în Django? NU.
        # Folosim mapping explicit pentru a păstra back-compat cu DEVICE_CHOICES existing.
        device_type = _yaml_id_to_device_type(dd_id)
        if not device_type:
            continue

        out[device_type] = resolve_capabilities(caps)

    logger.info("dd_loader: loaded capabilities for %d device_type(s) from %s",
                len(out), dd_dir)
    return out


# Mapping YAML id → Django device_type. Necesar pentru că YAML id e mai
# descriptiv (huawei_sun2000_3phase) decât DEVICE_CHOICES short (sun2000).
_YAML_ID_TO_DEVICE_TYPE: dict[str, str] = {
    "huawei_sun2000_3phase": "sun2000",
    "nous_a1t": "nous_at",
    "shelly_em": "shelly_em",
    "zigbee_temperature": "zigbee_sensor",
}


def _yaml_id_to_device_type(yaml_id: str) -> str | None:
    """Convertește id-ul DD din YAML la device_type-ul folosit în Django DEVICE_CHOICES."""
    return _YAML_ID_TO_DEVICE_TYPE.get(yaml_id)


def get_capabilities_for_device_type(device_type: str) -> list[str]:
    """Returnează lista resolved capabilities pentru un device_type sau [] dacă nu există DD."""
    return _load_dd_directory().get(device_type, [])


def reload_dd_cache() -> None:
    """Forțează reload-ul cache-ului DD (folosit de management command)."""
    _load_dd_directory.cache_clear()
