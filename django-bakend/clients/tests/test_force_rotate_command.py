"""Tests pentru management command force_rotate_mqtt_credentials."""
import io

import pytest
from django.contrib.auth import get_user_model
from django.contrib.auth.hashers import check_password, make_password
from django.core.management import call_command
from django.core.management.base import CommandError

from clients.models import Device
from tenants.models import Tenant


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="Acme", slug="acme")


@pytest.fixture
def owner(db, tenant):
    return get_user_model().objects.create_user(username="alice", password="pw", prenume="Alice")


@pytest.fixture
def device_no_hash(db, owner, tenant):
    return Device.objects.create(
        client=owner, tenant=tenant,
        serial_number="DEV-NOHASH",
        device_type="auto_detected",
    )


@pytest.fixture
def device_with_hash(db, owner, tenant):
    return Device.objects.create(
        client=owner, tenant=tenant,
        serial_number="DEV-WITHHASH",
        device_type="auto_detected",
        mqtt_password_hash=make_password("preexisting", hasher="bcrypt_sha256"),
    )


def test_dry_run_does_not_modify(device_no_hash):
    out = io.StringIO()
    call_command("force_rotate_mqtt_credentials", "--dry-run", stdout=out)
    device_no_hash.refresh_from_db()
    assert device_no_hash.mqtt_password_hash == ""
    assert "DEV-NOHASH" in out.getvalue()


def test_all_rotates_only_devices_without_hash(device_no_hash, device_with_hash):
    out = io.StringIO()
    call_command("force_rotate_mqtt_credentials", "--all", stdout=out)
    device_no_hash.refresh_from_db()
    device_with_hash.refresh_from_db()
    assert device_no_hash.mqtt_password_hash != ""
    # Device-ul care avea deja hash nu trebuie rotat fără --include-existing
    assert check_password("preexisting", device_with_hash.mqtt_password_hash)


def test_include_existing_rotates_everyone(device_no_hash, device_with_hash):
    out = io.StringIO()
    call_command("force_rotate_mqtt_credentials", "--all", "--include-existing", stdout=out)
    device_with_hash.refresh_from_db()
    # Vechea parolă nu mai e validă
    assert not check_password("preexisting", device_with_hash.mqtt_password_hash)


def test_device_specific(device_no_hash, device_with_hash):
    out = io.StringIO()
    call_command("force_rotate_mqtt_credentials", "--device", "DEV-NOHASH", stdout=out)
    device_no_hash.refresh_from_db()
    device_with_hash.refresh_from_db()
    assert device_no_hash.mqtt_password_hash != ""
    # Celălalt e neatins
    assert check_password("preexisting", device_with_hash.mqtt_password_hash)


def test_device_nonexistent_raises(db):
    with pytest.raises(CommandError):
        call_command("force_rotate_mqtt_credentials", "--device", "DOES-NOT-EXIST")


def test_no_args_raises():
    with pytest.raises(CommandError):
        call_command("force_rotate_mqtt_credentials")


def test_password_in_output_is_valid_for_auth(device_no_hash):
    out = io.StringIO()
    call_command("force_rotate_mqtt_credentials", "--all", stdout=out)
    output = out.getvalue()
    # Caută "password=" în output ca să extragem parola plain
    line = [l for l in output.splitlines() if "DEV-NOHASH" in l and "password=" in l][0]
    plain = line.split("password=")[1].strip()
    device_no_hash.refresh_from_db()
    assert check_password(plain, device_no_hash.mqtt_password_hash)
