from django.contrib.auth import get_user_model
from django.utils import timezone
from rest_framework import serializers

from tenants.models import Tenant
from .models import Device, DeviceShadow, DeviceCommand
from .topic_templates import TOPIC_TEMPLATES


class DeviceSerializer(serializers.ModelSerializer):
    topics = serializers.SerializerMethodField()
    tenant = serializers.PrimaryKeyRelatedField(queryset=Tenant.objects.all(), required=False)
    client = serializers.PrimaryKeyRelatedField(queryset=get_user_model().objects.all(), required=False)
    # Faza 2.7: plan-ul tenantului inclus în listarea device-urilor (read-only).
    # Permite Go-ului să ruteze scrierile în Influx pe bucket-ul corect fără HTTP extra.
    tenant_plan = serializers.CharField(source="tenant.plan", read_only=True, default="free")

    class Meta:
        model = Device
        fields = [
            "id",
            "serial_number",
            "description",
            "device_type",
            "client",
            "tenant",
            "tenant_plan",
            "topics",
        ]
        # tenant is injected by DeviceViewSet.perform_create from the JWT, not from payload.
        # Auto-generated UniqueTogetherValidator(tenant, serial_number) would mark tenant as
        # required in input — disable it; the DB constraint still enforces uniqueness.
        validators = []

    def get_topics(self, obj):
        template_list = TOPIC_TEMPLATES.get(obj.device_type, [])
        return [t.format(serial=obj.serial_number) for t in template_list]


class DeviceShadowSerializer(serializers.ModelSerializer):
    delta = serializers.SerializerMethodField()

    class Meta:
        model = DeviceShadow
        fields = ["reported", "desired", "delta", "version", "updated_at"]
        read_only_fields = ["reported", "delta", "version", "updated_at"]

    def get_delta(self, obj):
        return {k: v for k, v in obj.desired.items() if obj.reported.get(k) != v}


class DeviceShadowReportedSerializer(serializers.ModelSerializer):
    class Meta:
        model = DeviceShadow
        fields = ["reported", "version", "updated_at"]
        read_only_fields = ["version", "updated_at"]


class DeviceCommandSerializer(serializers.ModelSerializer):
    timed_out = serializers.SerializerMethodField()

    class Meta:
        model = DeviceCommand
        fields = [
            "id", "action", "payload", "status", "result",
            "created_at", "sent_at", "executed_at", "timed_out",
        ]
        read_only_fields = ["id", "status", "result", "created_at", "sent_at", "executed_at"]

    def get_timed_out(self, obj):
        if obj.status == DeviceCommand.Status.SENT and obj.sent_at:
            from datetime import timedelta
            return obj.sent_at < timezone.now() - timedelta(minutes=5)
        return False

