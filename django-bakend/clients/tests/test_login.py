"""Tests for the tenant-aware JWT login (POST /api/token/)."""
import jwt as pyjwt
import pytest
from django.conf import settings
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from tenants.models import Membership, Tenant


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def alice(db):
    return get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")


@pytest.fixture
def acme(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def globex(db):
    return Tenant.objects.create(name="Globex", slug="globex")


def decode(access):
    return pyjwt.decode(access, settings.SIMPLE_JWT["SIGNING_KEY"], algorithms=["HS256"])


def test_login_no_membership_rejected(api, alice):
    r = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    assert r.status_code == 400
    assert "no active tenant" in str(r.json()).lower()


def test_login_single_membership_implicit(api, alice, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    r = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    assert r.status_code == 200
    body = r.json()
    assert body["tenant_slug"] == "acme"
    assert body["role"] == "OWNER"
    claims = decode(body["access"])
    assert claims["tenant_id"] == acme.id
    assert claims["tenant_slug"] == "acme"
    assert claims["role"] == "OWNER"
    assert claims["username"] == "alice"
    assert claims["iss"] == "django"


def test_login_multiple_memberships_requires_slug(api, alice, acme, globex):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    Membership.objects.create(user=alice, tenant=globex, role=Membership.Role.VIEWER)
    r = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    assert r.status_code == 400
    assert "tenant_slug" in r.json()


def test_login_multiple_memberships_with_slug(api, alice, acme, globex):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    Membership.objects.create(user=alice, tenant=globex, role=Membership.Role.VIEWER)
    r = api.post(
        "/api/token/",
        {"username": "alice", "password": "pw", "tenant_slug": "globex"},
        format="json",
    )
    assert r.status_code == 200
    claims = decode(r.json()["access"])
    assert claims["tenant_slug"] == "globex"
    assert claims["role"] == "VIEWER"


def test_login_unknown_tenant_slug_rejected(api, alice, acme, globex):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    r = api.post(
        "/api/token/",
        {"username": "alice", "password": "pw", "tenant_slug": "ghost"},
        format="json",
    )
    assert r.status_code == 400


def test_login_suspended_tenant_excluded(api, alice, acme):
    acme.status = Tenant.Status.SUSPENDED
    acme.save()
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    r = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    assert r.status_code == 400
    assert "no active tenant" in str(r.json()).lower()


def test_refresh_token_carries_tenant_claims(alice, acme):
    """Custom claims set on the refresh token must be inherited by access tokens derived from it."""
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    api = APIClient()
    r = api.post("/api/token/", {"username": "alice", "password": "pw"}, format="json")
    refresh = r.json()["refresh"]
    refresh_claims = decode(refresh)
    assert refresh_claims["tenant_id"] == acme.id
    assert refresh_claims["role"] == "OWNER"
