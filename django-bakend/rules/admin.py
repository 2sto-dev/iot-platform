import json

from django.contrib import admin
from django.utils.html import format_html

from .models import Rule, RuleExecution


class RuleAdmin(admin.ModelAdmin):
    list_display = ["name", "tenant", "trigger_stream_pattern", "enabled", "cooldown_seconds", "updated_at"]
    list_filter = ["enabled", "tenant"]
    search_fields = ["name", "tenant__name"]
    readonly_fields = ["created_at", "updated_at", "conditions_pretty", "actions_pretty"]
    list_editable = ["enabled"]
    ordering = ["tenant", "name"]

    fieldsets = [
        (None, {
            "fields": ["tenant", "name", "description", "enabled", "cooldown_seconds", "trigger_stream_pattern"],
        }),
        ("Conditions (DSL)", {
            "fields": ["conditions", "conditions_pretty"],
            "description": (
                'Arbore JSON cu operator AND/OR/NOT și frunze {field, op, value}. '
                'Operatori: eq, ne, gt, gte, lt, lte, in, not_in, contains, not_contains, regex, is_null, is_not_null, changed.'
            ),
        }),
        ("Actions", {
            "fields": ["actions", "actions_pretty"],
            "description": 'Listă JSON de acțiuni: downlink, notify, webhook, set_shadow.',
        }),
        ("Timestamps", {
            "fields": ["created_at", "updated_at"],
            "classes": ["collapse"],
        }),
    ]

    def conditions_pretty(self, obj):
        try:
            return format_html("<pre style='font-size:12px'>{}</pre>", json.dumps(obj.conditions, indent=2))
        except Exception:
            return "-"
    conditions_pretty.short_description = "Conditions (preview)"

    def actions_pretty(self, obj):
        try:
            return format_html("<pre style='font-size:12px'>{}</pre>", json.dumps(obj.actions, indent=2))
        except Exception:
            return "-"
    actions_pretty.short_description = "Actions (preview)"


class RuleExecutionAdmin(admin.ModelAdmin):
    list_display = ["rule_name", "tenant", "device_serial", "stream", "status", "triggered_at"]
    list_filter = ["status", "tenant", "stream"]
    search_fields = ["rule_name", "device_serial"]
    readonly_fields = [
        "rule", "rule_name", "tenant", "device_serial", "stream",
        "triggered_at", "status", "error_message",
        "conditions_snapshot", "actions_taken",
    ]
    ordering = ["-triggered_at"]

    def has_add_permission(self, request):
        return False

    def has_change_permission(self, request, obj=None):
        return False


admin.site.register(Rule, RuleAdmin)
admin.site.register(RuleExecution, RuleExecutionAdmin)
