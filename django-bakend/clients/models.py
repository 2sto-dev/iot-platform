from django.contrib.auth.models import AbstractUser
from django.db import models


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
    serial_number = models.CharField(max_length=100, unique=True)
    description = models.CharField(max_length=200, blank=True)
    device_type = models.CharField(max_length=20, choices=DEVICE_CHOICES)

    def __str__(self):
        return f"{self.serial_number} - {self.get_device_type_display()} ({self.client.username})"
