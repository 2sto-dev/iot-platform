from rest_framework import serializers

from .models import DeviceOTAStatus, Firmware, RolloutPlan


class FirmwareSerializer(serializers.ModelSerializer):
    class Meta:
        model = Firmware
        fields = [
            "id", "device_type", "version", "file_url",
            "checksum_sha256", "size_bytes", "release_notes", "created_at",
        ]
        read_only_fields = ["id", "created_at"]


class RolloutPlanSerializer(serializers.ModelSerializer):
    error_rate = serializers.FloatField(read_only=True)
    firmware_version = serializers.CharField(source="firmware.version", read_only=True)
    device_type = serializers.CharField(source="firmware.device_type", read_only=True)
    stats = serializers.SerializerMethodField()

    class Meta:
        model = RolloutPlan
        fields = [
            "id", "firmware", "firmware_version", "device_type",
            "status", "canary_percent", "current_percent", "target_percent",
            "step_percent", "error_threshold", "error_rate",
            "stats", "created_at", "started_at", "completed_at",
        ]
        read_only_fields = [
            "id", "firmware", "firmware_version", "device_type",
            "status", "current_percent", "error_rate", "stats",
            "created_at", "started_at", "completed_at",
        ]

    def get_stats(self, obj):
        qs = obj.device_statuses
        return {
            "total": qs.count(),
            "pending": qs.filter(status=DeviceOTAStatus.Status.PENDING).count(),
            "sent": qs.filter(status=DeviceOTAStatus.Status.SENT).count(),
            "success": qs.filter(status=DeviceOTAStatus.Status.SUCCESS).count(),
            "failed": qs.filter(status=DeviceOTAStatus.Status.FAILED).count(),
        }


class DeviceOTAStatusSerializer(serializers.ModelSerializer):
    serial_number = serializers.CharField(source="device.serial_number", read_only=True)

    class Meta:
        model = DeviceOTAStatus
        fields = ["id", "serial_number", "status", "error_message", "sent_at", "updated_at"]
        read_only_fields = ["id", "serial_number", "sent_at", "updated_at"]


class RolloutCreateSerializer(serializers.Serializer):
    firmware_id = serializers.IntegerField()
    canary_percent = serializers.IntegerField(min_value=1, max_value=100, default=5)
    target_percent = serializers.IntegerField(min_value=1, max_value=100, default=100)
    step_percent = serializers.IntegerField(min_value=1, max_value=100, default=10)
    error_threshold = serializers.FloatField(min_value=0.0, max_value=1.0, default=0.1)
