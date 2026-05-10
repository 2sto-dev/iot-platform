from django.db import models


class NotificationChannel(models.Model):
    """A delivery channel for rule-triggered notifications.

    config schema per type:
      webhook: {"url": str, "method": "POST", "headers": {k: v}}
      email:   {"to": ["addr@..."], "from_name": str}
      fcm:     {"token": str}   (device token)
               {"topic": str}  (FCM topic)
    """
    class Type(models.TextChoices):
        WEBHOOK = "webhook", "Webhook HTTP"
        EMAIL = "email", "Email"
        FCM = "fcm", "Firebase Cloud Messaging"

    tenant = models.ForeignKey(
        "tenants.Tenant",
        on_delete=models.CASCADE,
        related_name="notification_channels",
    )
    name = models.CharField(max_length=100)
    type = models.CharField(max_length=20, choices=Type.choices)
    config = models.JSONField(
        help_text="Type-specific config. See model docstring.",
    )
    enabled = models.BooleanField(default=True)
    created_at = models.DateTimeField(auto_now_add=True)

    class Meta:
        unique_together = ("tenant", "name")
        ordering = ["name"]

    def __str__(self):
        return f"{self.name} ({self.type})"


class NotificationEvent(models.Model):
    class Status(models.TextChoices):
        PENDING = "pending"
        SENT = "sent"
        FAILED = "failed"

    channel = models.ForeignKey(
        NotificationChannel,
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
        related_name="events",
    )
    channel_name = models.CharField(max_length=100)
    rule_execution_id = models.BigIntegerField(null=True, blank=True)
    title = models.CharField(max_length=200, blank=True)
    body = models.TextField()
    context = models.JSONField(default=dict)
    status = models.CharField(
        max_length=20,
        choices=Status.choices,
        default=Status.PENDING,
    )
    attempts = models.PositiveIntegerField(default=0)
    last_error = models.TextField(blank=True)
    created_at = models.DateTimeField(auto_now_add=True, db_index=True)
    sent_at = models.DateTimeField(null=True, blank=True)

    class Meta:
        ordering = ["-created_at"]
