"""Notification delivery: webhook, email, FCM.

Each send_* function updates the NotificationEvent status in-place.
Called from a background thread so the API response is not blocked.
"""
import json
import logging
import threading
from datetime import datetime, timezone

logger = logging.getLogger(__name__)


def _mark_sent(event):
    from .models import NotificationEvent
    NotificationEvent.objects.filter(pk=event.pk).update(
        status=NotificationEvent.Status.SENT,
        sent_at=datetime.now(timezone.utc),
        attempts=event.attempts + 1,
        last_error="",
    )


def _mark_failed(event, error: str):
    from .models import NotificationEvent
    NotificationEvent.objects.filter(pk=event.pk).update(
        status=NotificationEvent.Status.FAILED,
        attempts=event.attempts + 1,
        last_error=error[:500],
    )


def _send_webhook(event, channel):
    import requests
    cfg = channel.config or {}
    url = cfg.get("url", "")
    method = cfg.get("method", "POST").upper()
    headers = cfg.get("headers", {})
    headers.setdefault("Content-Type", "application/json")
    body = {"title": event.title, "body": event.body, "context": event.context}
    try:
        resp = requests.request(method, url, json=body, headers=headers, timeout=10)
        resp.raise_for_status()
        _mark_sent(event)
    except Exception as exc:
        _mark_failed(event, str(exc))
        logger.warning("webhook notification failed: %s", exc)


def _send_email(event, channel):
    from django.core.mail import send_mail
    from django.conf import settings
    cfg = channel.config or {}
    recipients = cfg.get("to", [])
    from_name = cfg.get("from_name", "IoT Platform")
    from_email = getattr(settings, "DEFAULT_FROM_EMAIL", "noreply@iot.local")
    if not recipients:
        _mark_failed(event, "No recipients configured.")
        return
    try:
        send_mail(
            subject=event.title or "IoT Alert",
            message=event.body,
            from_email=f"{from_name} <{from_email}>",
            recipient_list=recipients,
            fail_silently=False,
        )
        _mark_sent(event)
    except Exception as exc:
        _mark_failed(event, str(exc))
        logger.warning("email notification failed: %s", exc)


def _send_fcm(event, channel):
    import requests
    from django.conf import settings
    cfg = channel.config or {}
    server_key = getattr(settings, "FCM_SERVER_KEY", "")
    if not server_key:
        _mark_failed(event, "FCM_SERVER_KEY not configured in settings.")
        return

    payload = {
        "notification": {
            "title": event.title or "IoT Alert",
            "body": event.body,
        },
        "data": {k: str(v) for k, v in event.context.items()},
    }
    if "token" in cfg:
        payload["to"] = cfg["token"]
    elif "topic" in cfg:
        payload["to"] = f"/topics/{cfg['topic']}"
    else:
        _mark_failed(event, "FCM config requires 'token' or 'topic'.")
        return

    try:
        resp = requests.post(
            "https://fcm.googleapis.com/fcm/send",
            json=payload,
            headers={
                "Authorization": f"key={server_key}",
                "Content-Type": "application/json",
            },
            timeout=10,
        )
        resp.raise_for_status()
        result = resp.json()
        if result.get("failure", 0) > 0:
            _mark_failed(event, json.dumps(result.get("results", [])))
        else:
            _mark_sent(event)
    except Exception as exc:
        _mark_failed(event, str(exc))
        logger.warning("FCM notification failed: %s", exc)


def _dispatch(event_id):
    """Fetch event and channel, dispatch to correct sender."""
    from .models import NotificationEvent
    try:
        event = NotificationEvent.objects.select_related("channel").get(pk=event_id)
    except NotificationEvent.DoesNotExist:
        return
    channel = event.channel
    if channel is None or not channel.enabled:
        _mark_failed(event, "Channel disabled or deleted.")
        return

    if channel.type == "webhook":
        _send_webhook(event, channel)
    elif channel.type == "email":
        _send_email(event, channel)
    elif channel.type == "fcm":
        _send_fcm(event, channel)
    else:
        _mark_failed(event, f"Unknown channel type: {channel.type}")


def send_async(event_id: int):
    """Dispatch notification in a background daemon thread."""
    t = threading.Thread(target=_dispatch, args=(event_id,), daemon=True)
    t.start()
