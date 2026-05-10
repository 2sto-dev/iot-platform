from rest_framework import serializers

from .models import Rule, RuleExecution
from .validators import validate_condition_node, validate_actions


class RuleSerializer(serializers.ModelSerializer):
    class Meta:
        model = Rule
        fields = [
            "id", "name", "description",
            "trigger_stream_pattern",
            "conditions", "actions",
            "cooldown_seconds", "enabled",
            "created_at", "updated_at",
        ]
        read_only_fields = ["id", "created_at", "updated_at"]

    def validate_conditions(self, value):
        validate_condition_node(value)
        return value

    def validate_actions(self, value):
        validate_actions(value)
        return value

    def validate_trigger_stream_pattern(self, value):
        if not value or not value.strip():
            raise serializers.ValidationError("trigger_stream_pattern cannot be empty.")
        return value.strip()

    def validate(self, attrs):
        request = self.context.get("request")
        tenant = getattr(request, "tenant", None) if request else None
        if tenant and "name" in attrs:
            qs = Rule.objects.filter(tenant=tenant, name=attrs["name"])
            if self.instance:
                qs = qs.exclude(pk=self.instance.pk)
            if qs.exists():
                raise serializers.ValidationError({"name": "A rule with this name already exists for the tenant."})
        return attrs


class RuleExecutionSerializer(serializers.ModelSerializer):
    class Meta:
        model = RuleExecution
        fields = [
            "id", "rule", "rule_name", "device_serial", "stream",
            "triggered_at", "conditions_snapshot", "actions_taken",
            "status", "error_message",
        ]
        read_only_fields = fields
