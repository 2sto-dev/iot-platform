from rest_framework import serializers
from .models import AuditLog


class AuditLogSerializer(serializers.ModelSerializer):
    actor_username = serializers.SerializerMethodField()

    def get_actor_username(self, obj):
        return obj.actor.username if obj.actor_id else None

    class Meta:
        model = AuditLog
        fields = [
            "id", "action", "resource_type", "resource_id",
            "actor_username", "ip", "metadata", "ts",
        ]
        read_only_fields = fields
