from rest_framework import serializers
from .models import APIKey


class APIKeySerializer(serializers.ModelSerializer):
    """Safe serializer — never exposes key_hash."""
    created_by_username = serializers.SerializerMethodField()

    def get_created_by_username(self, obj):
        return obj.created_by.username if obj.created_by_id else None

    class Meta:
        model = APIKey
        fields = [
            "id", "name", "prefix", "scopes",
            "expires_at", "last_used_at", "revoked",
            "created_at", "created_by_username",
        ]
        read_only_fields = [
            "id", "prefix", "last_used_at", "revoked",
            "created_at", "created_by_username",
        ]


class APIKeyCreateSerializer(serializers.Serializer):
    """Input for creating a new key. Returns the plain key once."""
    name = serializers.CharField(max_length=100)
    scopes = serializers.ListField(child=serializers.CharField(), default=list)
    expires_at = serializers.DateTimeField(required=False, allow_null=True)


class APIKeyCreateResponseSerializer(APIKeySerializer):
    plain_key = serializers.CharField(read_only=True)

    class Meta(APIKeySerializer.Meta):
        fields = APIKeySerializer.Meta.fields + ["plain_key"]
