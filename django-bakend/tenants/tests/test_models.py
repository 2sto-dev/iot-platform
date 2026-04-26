"""Smoke tests for Tenant and Membership models — defaults, constraints, cascades."""
import pytest
from django.contrib.auth import get_user_model
from django.db import IntegrityError

from tenants.models import Membership, Tenant


@pytest.fixture
def alice(db):
    return get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")


@pytest.fixture
def bob(db):
    return get_user_model().objects.create_user(username="bob", password="pw", prenume="Bob")


@pytest.fixture
def acme(db):
    return Tenant.objects.create(name="Acme Corp", slug="acme-corp")


@pytest.fixture
def globex(db):
    return Tenant.objects.create(name="Globex", slug="globex")


def test_tenant_defaults(acme):
    assert acme.plan == Tenant.Plan.FREE
    assert acme.status == Tenant.Status.ACTIVE
    assert acme.created_at is not None
    assert acme.updated_at is not None


def test_tenant_str(acme):
    assert str(acme) == "Acme Corp (acme-corp)"


def test_tenant_slug_unique(acme):
    with pytest.raises(IntegrityError):
        Tenant.objects.create(name="Acme Other", slug="acme-corp")


def test_membership_default_role(alice, acme):
    m = Membership.objects.create(user=alice, tenant=acme)
    assert m.role == Membership.Role.VIEWER


def test_membership_unique_user_tenant(alice, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    with pytest.raises(IntegrityError):
        Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.ADMIN)


def test_user_can_belong_to_multiple_tenants(alice, acme, globex):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    Membership.objects.create(user=alice, tenant=globex, role=Membership.Role.VIEWER)
    assert alice.memberships.count() == 2


def test_tenant_can_have_multiple_members(alice, bob, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    Membership.objects.create(user=bob, tenant=acme, role=Membership.Role.OPERATOR)
    assert acme.memberships.count() == 2


def test_cascade_delete_tenant_removes_memberships(alice, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    acme.delete()
    assert Membership.objects.count() == 0


def test_cascade_delete_user_removes_memberships(alice, acme):
    Membership.objects.create(user=alice, tenant=acme, role=Membership.Role.OWNER)
    alice.delete()
    assert Membership.objects.count() == 0
