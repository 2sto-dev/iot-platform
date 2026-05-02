from django.db import models
from django.utils import timezone
from django.contrib.auth.hashers import make_password, check_password


class DeviceCredential(models.Model):
    class Status(models.TextChoices):
        ACTIVE = "active", "Active"
        REVOKED = "revoked", "Revoked"

    device = models.OneToOneField("clients.Device", on_delete=models.CASCADE, related_name="credential")
    secret_hash = models.CharField(max_length=256)
    status = models.CharField(max_length=20, choices=Status.choices, default=Status.ACTIVE)
    rotated_at = models.DateTimeField(default=timezone.now)
    created_at = models.DateTimeField(auto_now_add=True)
    updated_at = models.DateTimeField(auto_now=True)

    def set_secret(self, plain: str):
        self.secret_hash = make_password(plain)
        self.rotated_at = timezone.now()
        self.status = self.Status.ACTIVE

    def verify(self, plain: str) -> bool:
        return check_password(plain, self.secret_hash)

    def __str__(self) -> str:
        return f"cred:{self.device.serial_number} [{self.status}]"
