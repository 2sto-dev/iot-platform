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
        ("sun2000", "Huawei SUN2000"),
        ("esp32_bmp180", "ESP32 BMP180"),
        ("esp32_bmp280", "ESP32 BMP280"),
        ("esp32_bme280", "ESP32 BME280"),
        ("esp32_ms5611", "ESP32 MS5611"),
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
    # Faza 3.1: hash BCrypt al parolei MQTT per-device. Gol = auth fără parolă (compat).
    mqtt_password_hash = models.CharField(max_length=128, blank=True)
    # Faza 5: capabilities denormalizate din Device Definition YAML.
    # Populat la save() prin signals.py + management command sync_capabilities.
    # Lista include atât capabilities declarate cât și cele inherited (smart_plug -> relay + power_meter).
    capabilities = models.JSONField(default=list, blank=True)

    objects = TenantQuerySet.as_manager()

    class Meta:
        unique_together = ("tenant", "serial_number")

    def __str__(self):
        return f"{self.serial_number} - {self.get_device_type_display()} ({self.client.username})"

    def has_capability(self, cap: str) -> bool:
        """Verifică dacă device-ul are capability (declarat sau inherited)."""
        return cap in (self.capabilities or [])


class DeviceShadow(models.Model):
    """Faza 3.4 — starea dorită și raportată a unui device."""
    device = models.OneToOneField(Device, on_delete=models.CASCADE, related_name="shadow")
    reported = models.JSONField(default=dict)  # ultima stare raportată de device
    desired = models.JSONField(default=dict)   # starea dorită setată de operator
    version = models.PositiveIntegerField(default=0)
    updated_at = models.DateTimeField(auto_now=True)

    def __str__(self):
        return f"Shadow({self.device.serial_number})"


class DeviceCommand(models.Model):
    """Faza 3.3 — comenzi downlink cu ACK tracking."""

    class Status(models.TextChoices):
        QUEUED = "queued"
        SENT = "sent"
        EXECUTED = "executed"
        FAILED = "failed"

    device = models.ForeignKey(Device, on_delete=models.CASCADE, related_name="commands")
    tenant = models.ForeignKey("tenants.Tenant", on_delete=models.CASCADE)
    action = models.CharField(max_length=100)
    payload = models.JSONField(default=dict)
    status = models.CharField(max_length=20, choices=Status.choices, default=Status.QUEUED)
    result = models.JSONField(default=dict)
    created_at = models.DateTimeField(auto_now_add=True)
    sent_at = models.DateTimeField(null=True, blank=True)
    executed_at = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"Cmd({self.id}) {self.action} → {self.device.serial_number} [{self.status}]"
