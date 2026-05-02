import json
import pytest
from django.urls import reverse
from django.contrib.auth import get_user_model
from clients.models import Device
from tenants.models import Tenant
from provisioning.models import DeviceCredential

User = get_user_model()


@pytest.mark.django_db
class TestProvisioning:
    def setup_method(self):
        self.tenant = Tenant.objects.create(name="Acme", slug="acme")
        self.user = User.objects.create_superuser(username="admin", password="x", email="admin@example.com")
        self.device = Device.objects.create(
            client=self.user,
            tenant=self.tenant,
            serial_number="DEVX",
            description="",
            device_type="auto_detected",
        )

    def test_rotate_generates_secret_and_hash(self, client):
        client.force_login(self.user)
        url = reverse("provisioning:rotate", kwargs={"device_id": self.device.id})
        r = client.post(url)
        assert r.status_code == 200
        data = r.json()
        assert data["serial"] == self.device.serial_number
        assert "secret" in data and len(data["secret"]) > 0
        cred = DeviceCredential.objects.get(device=self.device)
        assert cred.secret_hash != data["secret"]
        assert cred.verify(data["secret"]) is True

    def test_mqtt_auth_allow_and_deny(self, client):
        # pregătește credentiale
        cred = DeviceCredential.objects.create(device=self.device, secret_hash="hash")
        cred.set_secret("mypassword")
        cred.save()

        url = reverse("provisioning:mqtt_auth")

        # allow corect
        r_ok = client.post(url, data=json.dumps({
            "username": self.device.serial_number,
            "password": "mypassword",
            "clientid": self.device.serial_number,
        }), content_type="application/json")
        assert r_ok.status_code == 200 and r_ok.json()["result"] == "allow"

        # deny parolă greșită
        r_bad = client.post(url, data=json.dumps({
            "username": self.device.serial_number,
            "password": "wrong",
            "clientid": self.device.serial_number,
        }), content_type="application/json")
        assert r_bad.status_code == 200 and r_bad.json()["result"] == "deny"

        # deny device inexistent
        r_none = client.post(url, data=json.dumps({
            "username": "NOPE",
            "password": "x",
            "clientid": "NOPE",
        }), content_type="application/json")
        assert r_none.status_code == 200 and r_none.json()["result"] == "deny"
