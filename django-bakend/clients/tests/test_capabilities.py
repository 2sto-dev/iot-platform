"""Tests for Faza 5: capabilities denormalization + ?capability= filter."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.dd_loader import resolve_capabilities, INHERITANCE_MAP
from clients.models import Device
from tenants.models import Membership, Tenant


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def acme(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def alice(db):
    return get_user_model().objects.create_user(
        username="alice", password="pw", prenume="Alice"
    )


@pytest.fixture
def alice_in_acme(alice, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    return alice


# ============================================================================
# Inheritance Resolve (mirror din Go)
# ============================================================================


def test_resolve_smart_plug():
    out = resolve_capabilities(["smart_plug"])
    assert "smart_plug" in out
    assert "relay" in out
    assert "power_meter" in out
    assert len(out) == 3


def test_resolve_hybrid_inverter():
    out = resolve_capabilities(["hybrid_inverter"])
    assert "hybrid_inverter" in out
    assert "inverter" in out
    assert "battery" in out
    assert len(out) == 3


def test_resolve_climate_sensor():
    out = resolve_capabilities(["climate_sensor"])
    assert set(out) == {"climate_sensor", "temperature_sensor", "humidity_sensor"}


def test_resolve_dedupe():
    # Declared deja conține parintele — nu duplicate.
    out = resolve_capabilities(["smart_plug", "relay"])
    assert len(out) == 3
    assert out.count("relay") == 1


def test_resolve_empty():
    assert resolve_capabilities([]) == []


def test_resolve_simple():
    # Capability fără inheritance.
    out = resolve_capabilities(["wifi"])
    assert out == ["wifi"]


def test_inheritance_map_has_expected_entries():
    """Validare că inheritance_map e sincron cu Go-side (smoke check)."""
    assert "smart_plug" in INHERITANCE_MAP
    assert "hybrid_inverter" in INHERITANCE_MAP
    assert "climate_sensor" in INHERITANCE_MAP


# ============================================================================
# Device.capabilities populare
# ============================================================================


@pytest.mark.django_db
def test_device_capabilities_populated_on_create(alice_in_acme, acme):
    """La creare device cu device_type cunoscut → capabilities populate via signal."""
    d = Device.objects.create(
        client=alice_in_acme,
        tenant=acme,
        serial_number="TEST-NOUS-001",
        device_type="nous_at",
    )
    # Pre-save signal ar trebui să fi populat capabilities din DD.
    assert d.capabilities, "capabilities should be auto-populated"
    assert "smart_plug" in d.capabilities
    assert "relay" in d.capabilities  # inherited
    assert "power_meter" in d.capabilities  # inherited


@pytest.mark.django_db
def test_device_capabilities_not_overridden_if_set(alice_in_acme, acme):
    """Dacă caller-ul setează capabilities explicit, semnalul nu suprascrie."""
    custom = ["custom_cap_1", "custom_cap_2"]
    d = Device.objects.create(
        client=alice_in_acme,
        tenant=acme,
        serial_number="TEST-CUSTOM-001",
        device_type="nous_at",
        capabilities=custom,
    )
    assert d.capabilities == custom


@pytest.mark.django_db
def test_device_capabilities_unknown_type(alice_in_acme, acme):
    """Device cu device_type fără DD mapping → capabilities rămân []."""
    d = Device.objects.create(
        client=alice_in_acme,
        tenant=acme,
        serial_number="TEST-UNKNOWN-001",
        device_type="auto_detected",  # nu există în _YAML_ID_TO_DEVICE_TYPE
    )
    assert d.capabilities == []


@pytest.mark.django_db
def test_has_capability_method(alice_in_acme, acme):
    d = Device.objects.create(
        client=alice_in_acme,
        tenant=acme,
        serial_number="TEST-SUN-001",
        device_type="sun2000",
    )
    assert d.has_capability("inverter")  # din hybrid_inverter inheritance
    assert d.has_capability("battery")
    assert not d.has_capability("nonexistent")


# ============================================================================
# API filter ?capability=
# ============================================================================


@pytest.mark.django_db
def test_api_filter_by_capability(api, alice_in_acme, acme):
    Device.objects.create(
        client=alice_in_acme, tenant=acme,
        serial_number="SUN-001", device_type="sun2000",
    )
    Device.objects.create(
        client=alice_in_acme, tenant=acme,
        serial_number="NOUS-001", device_type="nous_at",
    )

    # Login alice
    resp = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    assert resp.status_code == 200, resp.json()
    token = resp.json()["access"]
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")

    # Filter capability=inverter — doar SUN-001 (hybrid_inverter inherits inverter)
    resp = api.get("/api/devices/?capability=inverter")
    assert resp.status_code == 200, resp.json()
    serials = [d["serial_number"] for d in resp.json()]
    assert "SUN-001" in serials
    assert "NOUS-001" not in serials

    # Filter capability=relay — doar NOUS-001 (smart_plug inherits relay)
    resp = api.get("/api/devices/?capability=relay")
    assert resp.status_code == 200
    serials = [d["serial_number"] for d in resp.json()]
    assert "NOUS-001" in serials
    assert "SUN-001" not in serials

    # Filter capability=power_meter — AMBELE devices (sun2000 explicit + nous via smart_plug)
    resp = api.get("/api/devices/?capability=power_meter")
    serials = [d["serial_number"] for d in resp.json()]
    assert "SUN-001" in serials
    assert "NOUS-001" in serials


@pytest.mark.django_db
def test_capabilities_in_response(api, alice_in_acme, acme):
    """DeviceSerializer expune capabilities."""
    Device.objects.create(
        client=alice_in_acme, tenant=acme,
        serial_number="TEST-001", device_type="sun2000",
    )

    resp = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {resp.json()['access']}")

    resp = api.get("/api/devices/")
    assert resp.status_code == 200
    devices = resp.json()
    assert len(devices) == 1
    assert "capabilities" in devices[0]
    assert isinstance(devices[0]["capabilities"], list)
    assert "inverter" in devices[0]["capabilities"]


@pytest.mark.django_db
def test_unknown_capability_returns_empty(api, alice_in_acme, acme):
    Device.objects.create(
        client=alice_in_acme, tenant=acme,
        serial_number="X", device_type="sun2000",
    )
    resp = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {resp.json()['access']}")

    resp = api.get("/api/devices/?capability=nonexistent_cap")
    assert resp.status_code == 200
    assert resp.json() == []
