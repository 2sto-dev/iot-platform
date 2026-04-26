from django.contrib.auth.models import AbstractUser
from django.db import models

from tenants.managers import TenantQuerySet


class Client(AbstractUser):
    prenume = models.CharField(max_length=50)
    telefon = models.CharField(max_length=20, blank=True, null=True)

    def __str__(self):
        return f"{self.username} ({self.prenume})"


class Device(models.Model):
    DEVICE_CHOICES = [
        ("shelly_em", "Shelly EM"),
        ("nous_at", "Nous AT"),
        ("zigbee_sensor", "Zigbee Sensor"),
        ("auto_detected", "Auto Detected"),
    ]

    client = models.ForeignKey(Client, on_delete=models.CASCADE, related_name="devices")
    tenant = models.ForeignKey(
        "tenants.Tenant",
        on_delete=models.PROTECT,
        related_name="devices",
    )
    serial_number = models.CharField(max_length=100)
    description = models.CharField(max_length=200, blank=True)
    device_type = models.CharField(max_length=20, choices=DEVICE_CHOICES)

    objects = TenantQuerySet.as_manager()

    class Meta:
        unique_together = ("tenant", "serial_number")

    def __str__(self):
        return f"{self.serial_number} - {self.get_device_type_display()} ({self.client.username})"
