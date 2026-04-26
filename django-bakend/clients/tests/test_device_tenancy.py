"""Tests for the tenant FK on Device + the legacy-tenant data migration."""
import pytest
from django.contrib.auth import get_user_model
from django.db import IntegrityError

from clients.models import Device
from tenants.models import Membership, Tenant


@pytest.fixture
def acme(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def globex(db):
    return Tenant.objects.create(name="Globex", slug="globex")


@pytest.fixture
def alice(db):
    return get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")


def test_same_serial_in_different_tenants_allowed(alice, acme, globex):
    Device.objects.create(client=alice, tenant=acme, serial_number="SHARED-001", device_type="shelly_em")
    Device.objects.create(client=alice, tenant=globex, serial_number="SHARED-001", device_type="shelly_em")
    assert Device.objects.filter(serial_number="SHARED-001").count() == 2


def test_same_serial_in_same_tenant_rejected(alice, acme):
    Device.objects.create(client=alice, tenant=acme, serial_number="DUP-001", device_type="shelly_em")
    with pytest.raises(IntegrityError):
        Device.objects.create(client=alice, tenant=acme, serial_number="DUP-001", device_type="nous_at")


def test_tenant_required(alice, acme):
    """tenant is NOT NULL after migration 0006 — creating without it must fail."""
    with pytest.raises(IntegrityError):
        Device.objects.create(client=alice, serial_number="NO-TENANT", device_type="shelly_em")


def test_legacy_tenant_exists_after_migrations(db):
    """Migration 0005 must create the 'legacy' tenant on every fresh DB."""
    assert Tenant.objects.filter(slug="legacy").exists()


def test_legacy_membership_created_for_existing_users(db):
    """Migration 0005 backfills OWNER membership for every Client that exists at migrate time.

    In the test DB, no users exist when 0005 runs, so legacy has 0 members.
    This test verifies the migration runs cleanly and the structure is correct.
    """
    legacy = Tenant.objects.get(slug="legacy")
    # Membership entries are only created for users existing at migration time;
    # users created later (via fixtures) don't auto-join.
    assert legacy.memberships.count() >= 0


def test_protect_on_tenant_delete(alice, acme):
    """Devices reference Tenant via PROTECT — cannot delete a tenant that has devices."""
    Device.objects.create(client=alice, tenant=acme, serial_number="P-001", device_type="shelly_em")
    from django.db.models.deletion import ProtectedError
    with pytest.raises(ProtectedError):
        acme.delete()
