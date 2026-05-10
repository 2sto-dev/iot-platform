"""Tests pentru Faza 3.2 — Activation flow."""
import hashlib
import json
import secrets
from datetime import timedelta

import pytest
from django.contrib.auth import get_user_model
from django.contrib.auth.hashers import check_password
from django.test import Client as DjangoClient
from django.utils import timezone

from clients.models import Device
from provisioning.models import ActivationToken
from tenants.models import Membership, Tenant


@pytest.fixture
def http():
    return DjangoClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def owner(db, tenant):
    user = get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")
    Membership.objects.create(user=user, tenant=tenant, role=Membership.Role.OWNER)
    return user


@pytest.fixture
def device(db, owner, tenant):
    return Device.objects.create(
        client=owner, tenant=tenant, serial_number="SHELF001", device_type="shelly_em"
    )


def _make_token(device, hours=72):
    plain = secrets.token_urlsafe(32)
    token_hash = hashlib.sha256(plain.encode()).hexdigest()
    ActivationToken.objects.create(
        device=device,
        token_hash=token_hash,
        expires_at=timezone.now() + timedelta(hours=hours),
    )
    return plain


def _activate(http, serial, token, password="newpass123"):
    return http.post(
        "/api/provisioning/activate/",
        data=json.dumps({"serial_number": serial, "activation_token": token, "mqtt_password": password}),
        content_type="application/json",
    )


def test_activate_sets_password(http, device):
    token = _make_token(device)
    r = _activate(http, "SHELF001", token)
    assert r.status_code == 200
    assert r.json()["activated"] is True
    device.refresh_from_db()
    assert check_password("newpass123", device.mqtt_password_hash)


def test_token_single_use(http, device):
    token = _make_token(device)
    _activate(http, "SHELF001", token)
    r2 = _activate(http, "SHELF001", token)
    assert r2.status_code == 400
    assert "expired or already used" in r2.json()["error"]


def test_expired_token_rejected(http, device):
    plain = secrets.token_urlsafe(32)
    ActivationToken.objects.create(
        device=device,
        token_hash=hashlib.sha256(plain.encode()).hexdigest(),
        expires_at=timezone.now() - timedelta(hours=1),  # expirat
    )
    r = _activate(http, "SHELF001", plain)
    assert r.status_code == 400
    assert "expired or already used" in r.json()["error"]


def test_wrong_token_rejected(http, device):
    _make_token(device)
    r = _activate(http, "SHELF001", "wrong-token-value")
    assert r.status_code == 400
    assert "Invalid activation token" in r.json()["error"]


def test_wrong_serial_rejected(http, device):
    token = _make_token(device)
    r = _activate(http, "UNKNOWN_SERIAL", token)
    assert r.status_code == 400
    assert "not found" in r.json()["error"]


def test_short_password_rejected(http, device):
    token = _make_token(device)
    r = _activate(http, "SHELF001", token, password="short")
    assert r.status_code == 400
    assert "8 characters" in r.json()["error"]


def test_missing_fields_rejected(http, db):
    r = http.post(
        "/api/provisioning/activate/",
        data=json.dumps({"serial_number": "X"}),
        content_type="application/json",
    )
    assert r.status_code == 400
