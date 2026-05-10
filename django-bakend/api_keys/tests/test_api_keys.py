"""Tests for Faza 4.3 — API Keys."""
import pytest
from django.contrib.auth import get_user_model
from django.utils import timezone
from rest_framework.test import APIClient

from api_keys.models import APIKey
from tenants.models import Membership, Tenant

User = get_user_model()


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="KeyCo", slug="keyco")


@pytest.fixture
def other_tenant(db):
    return Tenant.objects.create(name="OtherCo", slug="otherco2")


def _make_user(username, tenant, role):
    user = User.objects.create_user(username=username, password="pw", prenume=username.title())
    Membership.objects.create(user=user, tenant=tenant, role=role)
    return user


@pytest.fixture
def owner(db, tenant):
    return _make_user("kowner", tenant, Membership.Role.OWNER)


@pytest.fixture
def viewer(db, tenant):
    return _make_user("kviewer", tenant, Membership.Role.VIEWER)


def _jwt(user, tenant):
    from rest_framework_simplejwt.tokens import AccessToken
    token = AccessToken.for_user(user)
    token["tenant_id"] = tenant.id
    token["role"] = Membership.objects.filter(user=user, tenant=tenant).first().role
    return str(token)


# ── Model tests ───────────────────────────────────────────────────────────────

class TestAPIKeyModel:
    def test_generate_returns_plain_key(self, db, tenant, owner):
        key, plain = APIKey.generate(tenant=tenant, name="test", created_by=owner)
        assert plain
        assert key.prefix == plain[:8]
        assert key.key_hash == APIKey.hash_key(plain)
        assert key.revoked is False

    def test_expired_key_invalid(self, db, tenant):
        key, _ = APIKey.generate(
            tenant=tenant, name="old",
            expires_at=timezone.now() - timezone.timedelta(seconds=1),
        )
        assert not key.is_valid()

    def test_revoked_key_invalid(self, db, tenant):
        key, _ = APIKey.generate(tenant=tenant, name="rev")
        key.revoked = True
        key.save()
        assert not key.is_valid()

    def test_valid_key(self, db, tenant):
        key, _ = APIKey.generate(tenant=tenant, name="good")
        assert key.is_valid()


# ── Authentication backend ────────────────────────────────────────────────────

@pytest.mark.django_db
class TestAPIKeyAuthentication:
    def test_auth_with_valid_key(self, api, tenant, owner):
        key, plain = APIKey.generate(tenant=tenant, name="auth-test", created_by=owner)
        api.credentials(HTTP_AUTHORIZATION=f"ApiKey {plain}")
        resp = api.get("/api/devices/")
        assert resp.status_code == 200

    def test_auth_with_invalid_key(self, api):
        api.credentials(HTTP_AUTHORIZATION="ApiKey notarealkey1234567890abcdef")
        resp = api.get("/api/devices/")
        assert resp.status_code == 401

    def test_auth_with_revoked_key(self, api, tenant, owner):
        key, plain = APIKey.generate(tenant=tenant, name="rev-test", created_by=owner)
        key.revoked = True
        key.save()
        api.credentials(HTTP_AUTHORIZATION=f"ApiKey {plain}")
        resp = api.get("/api/devices/")
        assert resp.status_code == 401

    def test_auth_with_expired_key(self, api, tenant, owner):
        key, plain = APIKey.generate(
            tenant=tenant, name="exp-test", created_by=owner,
            expires_at=timezone.now() - timezone.timedelta(hours=1),
        )
        api.credentials(HTTP_AUTHORIZATION=f"ApiKey {plain}")
        resp = api.get("/api/devices/")
        assert resp.status_code == 401

    def test_last_used_at_updated(self, api, tenant, owner):
        key, plain = APIKey.generate(tenant=tenant, name="lu-test", created_by=owner)
        assert key.last_used_at is None
        api.credentials(HTTP_AUTHORIZATION=f"ApiKey {plain}")
        api.get("/api/devices/")
        key.refresh_from_db()
        assert key.last_used_at is not None


# ── REST endpoint tests ───────────────────────────────────────────────────────

@pytest.mark.django_db
class TestAPIKeyEndpoints:
    def test_owner_can_create_key(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/api-keys/", {"name": "my-key", "scopes": ["read"]}, format="json")
        assert resp.status_code == 201
        assert "plain_key" in resp.data
        assert resp.data["prefix"] == resp.data["plain_key"][:8]

    def test_viewer_cannot_create_key(self, api, viewer, tenant):
        token = _jwt(viewer, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/api-keys/", {"name": "key"}, format="json")
        assert resp.status_code == 403

    def test_owner_can_list_keys(self, api, owner, tenant):
        APIKey.generate(tenant=tenant, name="k1")
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/api-keys/")
        assert resp.status_code == 200
        assert len(resp.data) >= 1

    def test_plain_key_not_in_list(self, api, owner, tenant):
        APIKey.generate(tenant=tenant, name="hidden")
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/api-keys/")
        for entry in resp.data:
            assert "plain_key" not in entry
            assert "key_hash" not in entry

    def test_owner_can_revoke_key(self, api, owner, tenant):
        key, _ = APIKey.generate(tenant=tenant, name="to-revoke")
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.delete(f"/api/v1/api-keys/{key.id}/")
        assert resp.status_code == 204
        key.refresh_from_db()
        assert key.revoked is True

    def test_viewer_cannot_revoke_key(self, api, viewer, tenant):
        key, _ = APIKey.generate(tenant=tenant, name="protected")
        token = _jwt(viewer, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.delete(f"/api/v1/api-keys/{key.id}/")
        assert resp.status_code == 403

    def test_tenant_isolation_revoke(self, api, owner, tenant, other_tenant, db):
        key, _ = APIKey.generate(tenant=other_tenant, name="other-key")
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.delete(f"/api/v1/api-keys/{key.id}/")
        assert resp.status_code == 404

    def test_revoked_keys_excluded_from_list(self, api, owner, tenant):
        key, _ = APIKey.generate(tenant=tenant, name="revoked-key")
        key.revoked = True
        key.save()
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/api-keys/")
        ids = [e["id"] for e in resp.data]
        assert key.id not in ids
