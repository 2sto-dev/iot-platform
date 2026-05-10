"""Faza 3.2 — Activation tokens pentru provisioning device-uri noi."""
from django.db import models
from django.utils import timezone


class ActivationToken(models.Model):
    device = models.OneToOneField(
        "clients.Device",
        on_delete=models.CASCADE,
        related_name="activation_token",
    )
    token_hash = models.CharField(max_length=64)  # SHA-256 hex al tokenului plain
    used = models.BooleanField(default=False)
    expires_at = models.DateTimeField()
    created_at = models.DateTimeField(auto_now_add=True)

    def is_valid(self):
        return not self.used and self.expires_at > timezone.now()

    def __str__(self):
        return f"ActivationToken({self.device.serial_number}, used={self.used})"
