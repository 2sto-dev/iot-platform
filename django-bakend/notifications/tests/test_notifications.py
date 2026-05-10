"""Tests for Faza 4.2 — Notifications."""
import pytest
from django.contrib.auth import get_user_model
from rest_framework.test import APIClient
from unittest.mock import patch

from notifications.models import NotificationChannel, NotificationEvent
from tenants.models import Membership, Tenant

User = get_user_model()


@pytest.fixture
def api():
    return APIClient()


@pytest.fixture
def tenant(db):
    return Tenant.objects.create(name="NotifCo", slug="notifco")


@pytest.fixture
def other_tenant(db):
    return Tenant.objects.create(name="Other2", slug="other-notif")


def _make_user(username, tenant, role):
    user = User.objects.create_user(username=username, password="pw", prenume=username.title())
    Membership.objects.create(user=user, tenant=tenant, role=role)
    return user


@pytest.fixture
def owner(db, tenant):
    return _make_user("notif_owner", tenant, Membership.Role.OWNER)


@pytest.fixture
def viewer(db, tenant):
    return _make_user("notif_viewer", tenant, Membership.Role.VIEWER)


def _jwt(user, tenant):
    from rest_framework_simplejwt.tokens import AccessToken
    token = AccessToken.for_user(user)
    token["tenant_id"] = tenant.id
    token["role"] = Membership.objects.filter(user=user, tenant=tenant).first().role
    return str(token)


@pytest.fixture
def webhook_channel(db, tenant):
    return NotificationChannel.objects.create(
        tenant=tenant,
        name="Slack webhook",
        type="webhook",
        config={"url": "https://hooks.slack.com/test", "method": "POST"},
    )


# ── Channel CRUD ──────────────────────────────────────────────────────────────

@pytest.mark.django_db
class TestChannelAPI:
    def test_owner_can_create_webhook_channel(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/notifications/channels/", {
            "name": "My Webhook",
            "type": "webhook",
            "config": {"url": "https://example.com/hook", "method": "POST"},
        }, format="json")
        assert resp.status_code == 201
        assert resp.data["type"] == "webhook"

    def test_owner_can_create_email_channel(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/notifications/channels/", {
            "name": "Ops email",
            "type": "email",
            "config": {"to": ["ops@example.com"]},
        }, format="json")
        assert resp.status_code == 201

    def test_viewer_cannot_create_channel(self, api, viewer, tenant):
        token = _jwt(viewer, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/notifications/channels/", {
            "name": "x", "type": "webhook",
            "config": {"url": "https://x.com"},
        }, format="json")
        assert resp.status_code == 403

    def test_webhook_missing_url_rejected(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/notifications/channels/", {
            "name": "bad", "type": "webhook", "config": {},
        }, format="json")
        assert resp.status_code == 400

    def test_email_missing_to_rejected(self, api, owner, tenant):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post("/api/v1/notifications/channels/", {
            "name": "bad-email", "type": "email", "config": {},
        }, format="json")
        assert resp.status_code == 400

    def test_tenant_isolation(self, api, owner, tenant, other_tenant, db):
        NotificationChannel.objects.create(
            tenant=other_tenant, name="other-ch", type="webhook",
            config={"url": "https://x.com"},
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/notifications/channels/")
        assert resp.status_code == 200
        names = [c["name"] for c in resp.data]
        assert "other-ch" not in names

    def test_owner_can_delete_channel(self, api, owner, tenant, webhook_channel):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.delete(f"/api/v1/notifications/channels/{webhook_channel.id}/")
        assert resp.status_code == 204


# ── Test notification send ────────────────────────────────────────────────────

@pytest.mark.django_db
class TestChannelTest:
    def test_test_endpoint_creates_event(self, api, owner, tenant, webhook_channel):
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        with patch("notifications.sender.send_async"):
            resp = api.post(f"/api/v1/notifications/channels/{webhook_channel.id}/test/")
        assert resp.status_code == 200
        assert NotificationEvent.objects.filter(channel=webhook_channel).exists()

    def test_test_wrong_tenant_404(self, api, owner, tenant, other_tenant, db):
        other_ch = NotificationChannel.objects.create(
            tenant=other_tenant, name="x", type="webhook", config={"url": "https://x.com"},
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.post(f"/api/v1/notifications/channels/{other_ch.id}/test/")
        assert resp.status_code == 404


# ── Event history ─────────────────────────────────────────────────────────────

@pytest.mark.django_db
class TestEventList:
    def test_owner_can_list_events(self, api, owner, tenant, webhook_channel):
        NotificationEvent.objects.create(
            channel=webhook_channel, channel_name=webhook_channel.name,
            title="T", body="B",
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/notifications/events/")
        assert resp.status_code == 200
        assert len(resp.data) >= 1

    def test_filter_by_status(self, api, owner, tenant, webhook_channel):
        NotificationEvent.objects.create(
            channel=webhook_channel, channel_name="x",
            title="T", body="B", status="sent",
        )
        NotificationEvent.objects.create(
            channel=webhook_channel, channel_name="x",
            title="T", body="B", status="failed",
        )
        token = _jwt(owner, tenant)
        api.credentials(HTTP_AUTHORIZATION=f"Bearer {token}")
        resp = api.get("/api/v1/notifications/events/?status=sent")
        assert resp.status_code == 200
        assert all(e["status"] == "sent" for e in resp.data)


# ── Sender unit tests ─────────────────────────────────────────────────────────

@pytest.mark.django_db
class TestSender:
    def test_webhook_sent_on_success(self, tenant, webhook_channel):
        from notifications.sender import _send_webhook
        event = NotificationEvent.objects.create(
            channel=webhook_channel, channel_name=webhook_channel.name,
            title="Alert", body="Power > 1000W", context={"power": 1200},
        )
        with patch("requests.request") as mock_req:
            mock_req.return_value.status_code = 200
            mock_req.return_value.raise_for_status = lambda: None
            _send_webhook(event, webhook_channel)
        event.refresh_from_db()
        assert event.status == "sent"

    def test_webhook_marks_failed_on_error(self, tenant, webhook_channel):
        from notifications.sender import _send_webhook
        event = NotificationEvent.objects.create(
            channel=webhook_channel, channel_name=webhook_channel.name,
            title="T", body="B",
        )
        with patch("requests.request", side_effect=Exception("conn refused")):
            _send_webhook(event, webhook_channel)
        event.refresh_from_db()
        assert event.status == "failed"
        assert "conn refused" in event.last_error

    def test_fcm_no_server_key_fails(self, tenant):
        from notifications.sender import _send_fcm
        ch = NotificationChannel.objects.create(
            tenant=tenant, name="fcm-test", type="fcm",
            config={"token": "device-fcm-token"},
        )
        event = NotificationEvent.objects.create(
            channel=ch, channel_name="fcm-test", title="T", body="B",
        )
        _send_fcm(event, ch)
        event.refresh_from_db()
        assert event.status == "failed"
        assert "FCM_SERVER_KEY" in event.last_error
