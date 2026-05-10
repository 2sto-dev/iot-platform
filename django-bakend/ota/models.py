from django.db import models

from clients.models import Device
from tenants.models import Tenant


class Firmware(models.Model):
    """Artefact de firmware pentru un tip de device și versiune dată."""

    DEVICE_TYPE_CHOICES = Device.DEVICE_CHOICES

    tenant = models.ForeignKey(Tenant, on_delete=models.PROTECT, related_name="firmwares")
    device_type = models.CharField(max_length=20, choices=DEVICE_TYPE_CHOICES)
    version = models.CharField(max_length=50)
    file_url = models.CharField(max_length=500)
    checksum_sha256 = models.CharField(max_length=64)
    size_bytes = models.PositiveIntegerField(default=0)
    release_notes = models.TextField(blank=True)
    created_by = models.ForeignKey(
        "clients.Client", on_delete=models.SET_NULL, null=True, blank=True
    )
    created_at = models.DateTimeField(auto_now_add=True)

    class Meta:
        unique_together = ("tenant", "device_type", "version")

    def __str__(self):
        return f"{self.device_type} v{self.version} ({self.tenant.slug})"


class RolloutPlan(models.Model):
    """Plan de rollout staged pentru un firmware."""

    class Status(models.TextChoices):
        PENDING = "pending"
        CANARY = "canary"
        ROLLING = "rolling"
        COMPLETE = "complete"
        ROLLED_BACK = "rolled_back"
        PAUSED = "paused"

    firmware = models.OneToOneField(Firmware, on_delete=models.CASCADE, related_name="rollout")
    tenant = models.ForeignKey(Tenant, on_delete=models.CASCADE, related_name="rollouts")
    status = models.CharField(max_length=20, choices=Status.choices, default=Status.PENDING)
    canary_percent = models.PositiveSmallIntegerField(default=5)
    current_percent = models.PositiveSmallIntegerField(default=0)
    target_percent = models.PositiveSmallIntegerField(default=100)
    step_percent = models.PositiveSmallIntegerField(default=10)
    error_threshold = models.FloatField(default=0.1)
    created_at = models.DateTimeField(auto_now_add=True)
    started_at = models.DateTimeField(null=True, blank=True)
    completed_at = models.DateTimeField(null=True, blank=True)

    def __str__(self):
        return f"Rollout({self.firmware}) [{self.status}]"

    @property
    def error_rate(self):
        total = self.device_statuses.filter(
            status__in=[DeviceOTAStatus.Status.SUCCESS, DeviceOTAStatus.Status.FAILED]
        ).count()
        if total == 0:
            return 0.0
        failed = self.device_statuses.filter(status=DeviceOTAStatus.Status.FAILED).count()
        return failed / total

    def should_auto_rollback(self):
        finished = self.device_statuses.filter(
            status__in=[DeviceOTAStatus.Status.SUCCESS, DeviceOTAStatus.Status.FAILED]
        ).count()
        # Evaluate only once we have at least one terminal result.
        return finished > 0 and self.error_rate > self.error_threshold


class DeviceOTAStatus(models.Model):
    """Starea OTA per-device pentru un rollout dat."""

    class Status(models.TextChoices):
        PENDING = "pending"
        SENT = "sent"
        DOWNLOADING = "downloading"
        INSTALLING = "installing"
        SUCCESS = "success"
        FAILED = "failed"

    device = models.ForeignKey(Device, on_delete=models.CASCADE, related_name="ota_statuses")
    firmware = models.ForeignKey(Firmware, on_delete=models.CASCADE, related_name="device_statuses")
    rollout = models.ForeignKey(
        RolloutPlan, on_delete=models.CASCADE, related_name="device_statuses"
    )
    status = models.CharField(max_length=20, choices=Status.choices, default=Status.PENDING)
    error_message = models.TextField(blank=True)
    sent_at = models.DateTimeField(null=True, blank=True)
    updated_at = models.DateTimeField(auto_now=True)

    class Meta:
        unique_together = ("device", "firmware")

    def __str__(self):
        return f"OTA({self.device.serial_number}, {self.firmware.version}) [{self.status}]"
