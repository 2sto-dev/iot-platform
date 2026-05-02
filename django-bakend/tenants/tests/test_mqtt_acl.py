import pytest
from django.urls import reverse
from clients.models import Client, Device
from tenants.models import Tenant


@pytest.mark.django_db
class TestMQTTACL:
    def setup_method(self):
        self.tenant = Tenant.objects.create(name="Acme", slug="acme")
        self.user = Client.objects.create_user(username="alice", password="x")
        self.device = Device.objects.create(
            client=self.user,
            tenant=self.tenant,
            serial_number="DEV1",
            description="",
            device_type="auto_detected",
        )

    def post_acl(self, client, **data):
        url = reverse("tenants:mqtt_acl")
        payload = {"clientid": "", "username": "", "topic": "", "action": "publish"}
        payload.update(data)
        return client.post(url, data=payload, content_type="application/json")

    def test_allow_legacy_shellies(self, client):
        r = self.post_acl(client, topic="shellies/DEV1/emeter/0/power")
        assert r.status_code == 200 and r.json()["result"] == "allow"

    def test_allow_tenant_scoped_up_with_clientid_match(self, client):
        topic = f"tenants/{self.tenant.id}/devices/{self.device.serial_number}/up/telemetry"
        r = self.post_acl(client, topic=topic, clientid=self.device.serial_number)
        assert r.status_code == 200 and r.json()["result"] == "allow"

    def test_allow_tenant_scoped_up_with_bridge_user(self, client, settings):
        settings.MQTT_BRIDGE_USER = "mqtt-bridge"
        topic = f"tenants/{self.tenant.id}/devices/{self.device.serial_number}/up/telemetry"
        r = self.post_acl(client, topic=topic, username="mqtt-bridge")
        assert r.status_code == 200 and r.json()["result"] == "allow"

    def test_deny_cross_tenant(self, client):
        other = Tenant.objects.create(name="Globex", slug="globex")
        topic = f"tenants/{other.id}/devices/{self.device.serial_number}/up/telemetry"
        r = self.post_acl(client, topic=topic, clientid=self.device.serial_number)
        assert r.status_code == 200 and r.json()["result"] == "deny"

    def test_subscribe_only_up(self, client):
        topic_up = f"tenants/{self.tenant.id}/devices/{self.device.serial_number}/up/telemetry"
        r1 = self.post_acl(client, topic=topic_up, action="subscribe")
        assert r1.status_code == 200 and r1.json()["result"] == "allow"
        topic_down = f"tenants/{self.tenant.id}/devices/{self.device.serial_number}/down/cmd"
        r2 = self.post_acl(client, topic=topic_down, action="subscribe")
        assert r2.status_code == 200 and r2.json()["result"] == "deny"
