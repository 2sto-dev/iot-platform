from rest_framework import serializers
from .models import NotificationChannel, NotificationEvent

_CHANNEL_CONFIG_KEYS = {
    "webhook": {"url"},
    "email": {"to"},
    "fcm": set(),
}


class NotificationChannelSerializer(serializers.ModelSerializer):
    class Meta:
        model = NotificationChannel
        fields = ["id", "name", "type", "config", "enabled", "created_at"]
        read_only_fields = ["id", "created_at"]

    def validate(self, data):
        channel_type = data.get("type") or (self.instance.type if self.instance else None)
        config = data.get("config") or (self.instance.config if self.instance else {})
        required = _CHANNEL_CONFIG_KEYS.get(channel_type, set())
        missing = required - set(config.keys())
        if missing:
            raise serializers.ValidationError(
                {"config": f"Missing required keys for {channel_type}: {sorted(missing)}"}
            )
        return data


class NotificationEventSerializer(serializers.ModelSerializer):
    class Meta:
        model = NotificationEvent
        fields = [
            "id", "channel", "channel_name",
            "title", "body", "context",
            "status", "attempts", "last_error",
            "created_at", "sent_at",
        ]
        read_only_fields = fields
