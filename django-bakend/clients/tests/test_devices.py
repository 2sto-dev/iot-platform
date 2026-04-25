"""Smoke tests for DeviceViewSet — verify auth and per-user filtering."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from clients.models import Device


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def user(db):
    User = get_user_model()
    return User.objects.create_user(username="alice", password="pw", prenume="Alice")


@pytest.fixture
def other_user(db):
    User = get_user_model()
    return User.objects.create_user(username="bob", password="pw", prenume="Bob")


@pytest.fixture
def alice_device(user):
    return Device.objects.create(
        client=user,
        serial_number="ALICE-001",
        description="Alice's device",
        device_type="shelly_em",
    )


@pytest.fixture
def bob_device(other_user):
    return Device.objects.create(
        client=other_user,
        serial_number="BOB-001",
        description="Bob's device",
        device_type="nous_at",
    )


def test_anonymous_request_rejected(api):
    response = api.get("/api/devices/")
    assert response.status_code == 401


def test_user_sees_only_own_devices(api, user, alice_device, bob_device):
    api.force_authenticate(user=user)
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = [d["serial_number"] for d in response.json()]
    assert serials == ["ALICE-001"]


def test_superuser_sees_all_devices(api, alice_device, bob_device):
    User = get_user_model()
    su = User.objects.create_superuser(username="root", password="pw", prenume="Root")
    api.force_authenticate(user=su)
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = sorted(d["serial_number"] for d in response.json())
    assert serials == ["ALICE-001", "BOB-001"]


def test_service_account_sees_all_devices(api, alice_device, bob_device):
    """User cu perm clients.view_device (service account, ex. iot-ingest) vede tot."""
    from django.contrib.auth.models import Permission
    from django.contrib.contenttypes.models import ContentType

    User = get_user_model()
    svc = User.objects.create_user(username="iot-ingest", password="pw", prenume="Ingest")
    ct = ContentType.objects.get_for_model(Device)
    svc.user_permissions.add(Permission.objects.get(content_type=ct, codename="view_device"))

    api.force_authenticate(user=svc)
    response = api.get("/api/devices/")
    assert response.status_code == 200
    serials = sorted(d["serial_number"] for d in response.json())
    assert serials == ["ALICE-001", "BOB-001"]
