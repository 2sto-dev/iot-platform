from django.contrib.auth import get_user_model
from rest_framework import serializers

from tenants.models import Tenant
from .models import Device
from .topic_templates import TOPIC_TEMPLATES


class DeviceSerializer(serializers.ModelSerializer):
    topics = serializers.SerializerMethodField()
    tenant = serializers.PrimaryKeyRelatedField(queryset=Tenant.objects.all(), required=False)
    client = serializers.PrimaryKeyRelatedField(queryset=get_user_model().objects.all(), required=False)

    class Meta:
        model = Device
        fields = [
            "id",
            "serial_number",
            "description",
            "device_type",
            "client",
            "tenant",
            "topics",
        ]
        # tenant is injected by DeviceViewSet.perform_create from the JWT, not from payload.
        # Auto-generated UniqueTogetherValidator(tenant, serial_number) would mark tenant as
        # required in input — disable it; the DB constraint still enforces uniqueness.
        validators = []

    def get_topics(self, obj):
        template_list = TOPIC_TEMPLATES.get(obj.device_type, [])
        return [t.format(serial=obj.serial_number) for t in template_list]

