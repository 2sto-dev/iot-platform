"""Teste pentru Faza 3.5 — OTA service (firmware, rollout staged, rollback auto)."""
from unittest.mock import patch

import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.models import Device
from ota.models import DeviceOTAStatus, Firmware, RolloutPlan
from tenants.models import Membership, Tenant


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def owner(db, tenant):
    user = get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")
    Membership.objects.create(user=user, tenant=tenant, role=Membership.Role.OWNER)
    return user


@pytest.fixture
def viewer(db, tenant):
    user = get_user_model().objects.create_user(username="viewer1", password="pw", prenume="Viewer")
    Membership.objects.create(user=user, tenant=tenant, role=Membership.Role.VIEWER)
    return user


@pytest.fixture
def service_account(db):
    return get_user_model().objects.create_superuser(
        username="svc", password="svc-pass", prenume="Service"
    )


@pytest.fixture
def devices(db, owner, tenant):
    return [
        Device.objects.create(
            client=owner, tenant=tenant,
            serial_number=f"DEV{i:03d}", device_type="shelly_em"
        )
        for i in range(10)
    ]


@pytest.fixture
def firmware(db, tenant, owner):
    return Firmware.objects.create(
        tenant=tenant,
        device_type="shelly_em",
        version="2.0.0",
        file_url="https://storage.example.com/fw/shelly_2.0.0.bin",
        checksum_sha256="abc123" * 10 + "abcd",
        size_bytes=512000,
        created_by=owner,
    )


def _login(api, username, password="pw", tenant_slug=None):
    payload = {"username": username, "password": password}
    if tenant_slug:
        payload["tenant_slug"] = tenant_slug
    r = api.post("/api/token/", payload, format="json")
    assert r.status_code == 200, r.json()
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")


# ── Firmware CRUD ─────────────────────────────────────────────────────────────

def test_create_firmware(api, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    r = api.post("/api/ota/firmware/", {
        "device_type": "shelly_em",
        "version": "1.9.0",
        "file_url": "https://example.com/fw.bin",
        "checksum_sha256": "a" * 64,
        "size_bytes": 100000,
    }, format="json")
    assert r.status_code == 201
    assert r.json()["version"] == "1.9.0"


def test_viewer_cannot_create_firmware(api, viewer, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "viewer1", tenant_slug="acme")
    r = api.post("/api/ota/firmware/", {
        "device_type": "shelly_em",
        "version": "1.9.0",
        "file_url": "https://example.com/fw.bin",
        "checksum_sha256": "a" * 64,
    }, format="json")
    assert r.status_code == 403


def test_list_firmware_scoped_to_tenant(api, firmware, owner, tenant, db, settings):
    settings.MQTT_BROKER = ""
    other_tenant = Tenant.objects.create(name="Other", slug="other")
    other_user = get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")
    Membership.objects.create(user=other_user, tenant=other_tenant, role=Membership.Role.OWNER)
    Firmware.objects.create(
        tenant=other_tenant, device_type="shelly_em", version="1.0.0",
        file_url="https://other.com/fw.bin", checksum_sha256="b" * 64,
    )
    _login(api, "alice", tenant_slug="acme")
    r = api.get("/api/ota/firmware/")
    assert r.status_code == 200
    versions = [f["version"] for f in r.json()]
    assert "2.0.0" in versions
    assert "1.0.0" not in versions


# ── Rollout ───────────────────────────────────────────────────────────────────

def test_create_rollout_starts_canary(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    with patch("ota.views._publish_ota_command"):
        r = api.post("/api/ota/rollouts/", {
            "firmware_id": firmware.id,
            "canary_percent": 30,
            "target_percent": 100,
            "step_percent": 10,
            "error_threshold": 0.1,
        }, format="json")
    assert r.status_code == 201
    data = r.json()
    assert data["status"] == "canary"
    assert data["current_percent"] == 30
    assert DeviceOTAStatus.objects.filter(firmware=firmware).count() >= 1


def test_cannot_create_duplicate_rollout(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    with patch("ota.views._publish_ota_command"):
        api.post("/api/ota/rollouts/", {"firmware_id": firmware.id}, format="json")
        r2 = api.post("/api/ota/rollouts/", {"firmware_id": firmware.id}, format="json")
    assert r2.status_code == 400


def test_advance_rollout_rolling(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    with patch("ota.views._publish_ota_command"):
        r = api.post("/api/ota/rollouts/", {
            "firmware_id": firmware.id,
            "canary_percent": 10,
            "step_percent": 20,
        }, format="json")
        rollout_id = r.json()["id"]
        r2 = api.post(f"/api/ota/rollouts/{rollout_id}/advance/")
    assert r2.status_code == 200
    assert r2.json()["status"] in {"rolling", "complete"}
    assert r2.json()["current_percent"] > 10


def test_advance_auto_rollback_on_errors(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    with patch("ota.views._publish_ota_command"):
        r = api.post("/api/ota/rollouts/", {
            "firmware_id": firmware.id,
            "canary_percent": 50,
            "error_threshold": 0.1,
        }, format="json")
    rollout_id = r.json()["id"]
    rollout = RolloutPlan.objects.get(pk=rollout_id)

    # Marchează toate device-urile trimise ca FAILED → error_rate = 100%
    DeviceOTAStatus.objects.filter(rollout=rollout).update(
        status=DeviceOTAStatus.Status.FAILED
    )

    with patch("ota.views._publish_ota_command"):
        r2 = api.post(f"/api/ota/rollouts/{rollout_id}/advance/")
    assert r2.status_code == 200
    rollout.refresh_from_db()
    assert rollout.status == RolloutPlan.Status.ROLLED_BACK


def test_manual_rollback(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    with patch("ota.views._publish_ota_command"):
        r = api.post("/api/ota/rollouts/", {"firmware_id": firmware.id}, format="json")
    rollout_id = r.json()["id"]
    r2 = api.post(f"/api/ota/rollouts/{rollout_id}/rollback/")
    assert r2.status_code == 200
    assert r2.json()["status"] == "rolled_back"


def test_pause_rollout(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    with patch("ota.views._publish_ota_command"):
        r = api.post("/api/ota/rollouts/", {"firmware_id": firmware.id}, format="json")
    rollout_id = r.json()["id"]
    r2 = api.post(f"/api/ota/rollouts/{rollout_id}/pause/")
    assert r2.status_code == 200
    assert r2.json()["status"] == "paused"


# ── Device OTA status update ──────────────────────────────────────────────────

def test_device_reports_ota_success(api, firmware, devices, owner, tenant, service_account, settings):
    settings.MQTT_BROKER = ""
    device = devices[0]
    rollout = RolloutPlan.objects.create(
        firmware=firmware, tenant=tenant,
        status=RolloutPlan.Status.CANARY, current_percent=10,
    )
    ota_status = DeviceOTAStatus.objects.create(
        device=device, firmware=firmware, rollout=rollout,
        status=DeviceOTAStatus.Status.SENT,
    )

    _login(api, "svc", password="svc-pass")
    r = api.patch(
        f"/api/ota/devices/{device.serial_number}/status/",
        {"firmware_id": firmware.id, "status": "success"},
        format="json",
    )
    assert r.status_code == 200
    ota_status.refresh_from_db()
    assert ota_status.status == DeviceOTAStatus.Status.SUCCESS


def test_device_reports_ota_failed_triggers_auto_rollback(
    api, firmware, devices, owner, tenant, service_account, settings
):
    settings.MQTT_BROKER = ""
    rollout = RolloutPlan.objects.create(
        firmware=firmware, tenant=tenant,
        status=RolloutPlan.Status.CANARY, current_percent=50,
        error_threshold=0.1,
    )
    # 8 succese, 1 sent → raportăm FAILED → total=9, failed=1 → 11% > 10% → rollback
    for i, d in enumerate(devices[:8]):
        DeviceOTAStatus.objects.create(
            device=d, firmware=firmware, rollout=rollout,
            status=DeviceOTAStatus.Status.SUCCESS,
        )
    DeviceOTAStatus.objects.create(
        device=devices[9], firmware=firmware, rollout=rollout,
        status=DeviceOTAStatus.Status.SENT,
    )

    _login(api, "svc", password="svc-pass")
    r = api.patch(
        f"/api/ota/devices/{devices[9].serial_number}/status/",
        {"firmware_id": firmware.id, "status": "failed", "error_message": "checksum mismatch"},
        format="json",
    )
    assert r.status_code == 200
    rollout.refresh_from_db()
    assert rollout.status == RolloutPlan.Status.ROLLED_BACK


def test_device_ota_status_requires_service_account(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    _login(api, "alice", tenant_slug="acme")
    r = api.patch(
        f"/api/ota/devices/{devices[0].serial_number}/status/",
        {"firmware_id": firmware.id, "status": "success"},
        format="json",
    )
    assert r.status_code == 403


def test_device_ota_history(api, firmware, devices, owner, tenant, settings):
    settings.MQTT_BROKER = ""
    rollout = RolloutPlan.objects.create(
        firmware=firmware, tenant=tenant,
        status=RolloutPlan.Status.CANARY, current_percent=10,
    )
    DeviceOTAStatus.objects.create(
        device=devices[0], firmware=firmware, rollout=rollout,
        status=DeviceOTAStatus.Status.SUCCESS,
    )
    _login(api, "alice", tenant_slug="acme")
    r = api.get(f"/api/devices/{devices[0].id}/ota/")
    assert r.status_code == 200
    assert len(r.json()) == 1
    assert r.json()[0]["status"] == "success"
