from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):
    dependencies = [("clients", "0008_deviceshadow")]

    operations = [
        migrations.CreateModel(
            name="DeviceCommand",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("action", models.CharField(max_length=100)),
                ("payload", models.JSONField(default=dict)),
                ("status", models.CharField(
                    choices=[
                        ("queued", "Queued"),
                        ("sent", "Sent"),
                        ("executed", "Executed"),
                        ("failed", "Failed"),
                    ],
                    default="queued",
                    max_length=20,
                )),
                ("result", models.JSONField(default=dict)),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                ("sent_at", models.DateTimeField(blank=True, null=True)),
                ("executed_at", models.DateTimeField(blank=True, null=True)),
                ("device", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="commands",
                    to="clients.device",
                )),
                ("tenant", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    to="tenants.tenant",
                )),
            ],
        ),
    ]
