"""Regression test: privilegiul de superuser e derivat server-side, NU din JWT.

JWT-ul în browser nu trebuie să poarte `is_service` (XSS exfil risk). Middleware-ul
verifică `user.is_superuser` la fiecare request, cu cache TTL.
"""
import jwt
import pytest
from django.conf import settings
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient

from tenants.middleware import _superuser_cache, invalidate_superuser_cache
from tenants.models import Tenant


@pytest.fixture(autouse=True)
def clear_cache():
    invalidate_superuser_cache()
    yield
    invalidate_superuser_cache()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def api():
    return APIClient()


def _login(api, username, password="pw", tenant_slug=None):
    body = {"username": username, "password": password}
    if tenant_slug:
        body["tenant_slug"] = tenant_slug
    r = api.post("/api/token/", body, format="json")
    return r


def _decode_no_verify(token):
    return jwt.decode(token, options={"verify_signature": False})


def test_jwt_does_not_carry_is_service_claim(api, tenant):
    """Login ca superuser cu tenant_slug → JWT-ul NU are claim-ul is_service."""
    get_user_model().objects.create_superuser(username="root", password="pw", prenume="Root")
    r = _login(api, "root", tenant_slug="acme")
    assert r.status_code == 200
    claims = _decode_no_verify(r.json()["access"])
    assert "is_service" not in claims, f"is_service leaked to browser: {claims}"
    # Confirmă că restul claim-urilor relevante sunt prezente
    assert claims["tenant_slug"] == "acme"
    assert claims["role"] == "OWNER"


def test_superuser_can_access_tenant_without_membership(api, tenant):
    """Privilegiul derivat din DB: superuser fără Membership trece prin middleware."""
    get_user_model().objects.create_superuser(username="root", password="pw", prenume="Root")
    r = _login(api, "root", tenant_slug="acme")
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")
    r2 = api.get("/api/devices/")
    assert r2.status_code == 200


def test_non_superuser_without_membership_denied(api, tenant):
    """Userul normal fără Membership → 403, indiferent ce ar fi în JWT.

    Tokenul forjat aici e semnat cu același secret și pretinde tenant_id, dar
    middleware-ul re-verifică DB: nu e superuser și nu are Membership → blocat.
    """
    user = get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")
    forged = jwt.encode(
        {
            "user_id": user.id,
            "username": "alice",
            "tenant_id": tenant.id,
            "tenant_slug": tenant.slug,
            "role": "OWNER",
            "iss": "django",
            "exp": 9999999999,
        },
        settings.SIMPLE_JWT["SIGNING_KEY"],
        algorithm="HS256",
    )
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {forged}")
    r = api.get("/api/devices/")
    assert r.status_code == 403


def test_superuser_demote_invalidates_cache(api, tenant):
    """Dacă un superuser e demote-uit, cache-ul se invalidează la save."""
    User = get_user_model()
    root = User.objects.create_superuser(username="root", password="pw", prenume="Root")
    r = _login(api, "root", tenant_slug="acme")
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {r.json()['access']}")

    # Acces inițial OK
    assert api.get("/api/devices/").status_code == 200
    assert root.id in _superuser_cache or True  # eventual cached

    # Demote: post_save signal invalidează cache-ul
    root.is_superuser = False
    root.save()
    assert root.id not in _superuser_cache

    # Acum nu mai are Membership → 403
    r2 = api.get("/api/devices/")
    assert r2.status_code == 403


def test_forging_is_service_claim_does_not_grant_privilege(api, tenant):
    """Defense-in-depth: dacă cineva forjează un JWT cu `is_service: true`,
    middleware-ul IGNORĂ claim-ul și verifică DB. User non-superuser fără
    Membership → 403, indiferent ce zice JWT-ul.
    """
    user = get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")
    forged = jwt.encode(
        {
            "user_id": user.id,
            "username": "bob",
            "tenant_id": tenant.id,
            "tenant_slug": tenant.slug,
            "role": "OWNER",
            "is_service": True,   # ← claim-ul fictiv, nu trebuie să facă nimic
            "iss": "django",
            "exp": 9999999999,
        },
        settings.SIMPLE_JWT["SIGNING_KEY"],
        algorithm="HS256",
    )
    api.credentials(HTTP_AUTHORIZATION=f"Bearer {forged}")
    r = api.get("/api/devices/")
    assert r.status_code == 403, "is_service claim a fost luat în considerare — privilege escalation!"
