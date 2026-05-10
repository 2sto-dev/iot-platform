from django.db import models


class Rule(models.Model):
    """A tenant-scoped automation rule.

    conditions: condition DSL tree — see validators.py for schema.
    actions:    list of action objects (downlink/notify/webhook/set_shadow).
    trigger_stream_pattern: stream name(s) that activate this rule.
        - "*"             → any stream
        - "telemetry"     → exact match
        - "telemetry,emeter" → comma-separated list (match any)
    cooldown_seconds: minimum interval between consecutive firings for the
        same rule + device pair. Tracked in Redis.
    """
    tenant = models.ForeignKey(
        "tenants.Tenant",
        on_delete=models.CASCADE,
        related_name="rules",
    )
    name = models.CharField(max_length=100)
    description = models.TextField(blank=True)
    trigger_stream_pattern = models.CharField(
        max_length=200,
        default="*",
        help_text='Stream name(s): "*", "telemetry", "telemetry,emeter"',
    )
    conditions = models.JSONField(
        help_text="Condition DSL tree. See API docs for schema.",
    )
    actions = models.JSONField(
        help_text="List of actions: downlink, notify, webhook, set_shadow.",
    )
    cooldown_seconds = models.PositiveIntegerField(
        default=60,
        help_text="Min seconds between consecutive firings for same device.",
    )
    enabled = models.BooleanField(default=True)
    created_at = models.DateTimeField(auto_now_add=True)
    updated_at = models.DateTimeField(auto_now=True)

    class Meta:
        unique_together = ("tenant", "name")
        ordering = ["name"]

    def __str__(self):
        return f"Rule({self.name})[tenant={self.tenant_id}]"


class RuleExecution(models.Model):
    """Audit trail — one record per rule firing."""

    class Status(models.TextChoices):
        TRIGGERED = "triggered"
        COOLDOWN = "cooldown_skipped"
        ERROR = "error"

    rule = models.ForeignKey(
        Rule,
        on_delete=models.SET_NULL,
        null=True,
        blank=True,
        related_name="executions",
    )
    rule_name = models.CharField(max_length=100)
    tenant = models.ForeignKey(
        "tenants.Tenant",
        on_delete=models.CASCADE,
        related_name="rule_executions",
    )
    device_serial = models.CharField(max_length=100)
    stream = models.CharField(max_length=50)
    triggered_at = models.DateTimeField(auto_now_add=True, db_index=True)
    conditions_snapshot = models.JSONField(default=dict)
    actions_taken = models.JSONField(default=list)
    status = models.CharField(
        max_length=20,
        choices=Status.choices,
        default=Status.TRIGGERED,
    )
    error_message = models.TextField(blank=True)

    class Meta:
        ordering = ["-triggered_at"]
