from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):
    initial = True
    dependencies = [
        ("tenants", "0001_initial"),
    ]

    operations = [
        migrations.CreateModel(
            name="Rule",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("name", models.CharField(max_length=100)),
                ("description", models.TextField(blank=True)),
                ("trigger_stream_pattern", models.CharField(default="*", max_length=200)),
                ("conditions", models.JSONField()),
                ("actions", models.JSONField()),
                ("cooldown_seconds", models.PositiveIntegerField(default=60)),
                ("enabled", models.BooleanField(default=True)),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                ("updated_at", models.DateTimeField(auto_now=True)),
                ("tenant", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="rules",
                    to="tenants.tenant",
                )),
            ],
            options={"ordering": ["name"], "unique_together": {("tenant", "name")}},
        ),
        migrations.CreateModel(
            name="RuleExecution",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("rule_name", models.CharField(max_length=100)),
                ("device_serial", models.CharField(max_length=100)),
                ("stream", models.CharField(max_length=50)),
                ("triggered_at", models.DateTimeField(auto_now_add=True, db_index=True)),
                ("conditions_snapshot", models.JSONField(default=dict)),
                ("actions_taken", models.JSONField(default=list)),
                ("status", models.CharField(
                    choices=[("triggered", "Triggered"), ("cooldown_skipped", "Cooldown"), ("error", "Error")],
                    default="triggered",
                    max_length=20,
                )),
                ("error_message", models.TextField(blank=True)),
                ("rule", models.ForeignKey(
                    blank=True,
                    null=True,
                    on_delete=django.db.models.deletion.SET_NULL,
                    related_name="executions",
                    to="rules.rule",
                )),
                ("tenant", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="rule_executions",
                    to="tenants.tenant",
                )),
            ],
            options={"ordering": ["-triggered_at"]},
        ),
    ]
