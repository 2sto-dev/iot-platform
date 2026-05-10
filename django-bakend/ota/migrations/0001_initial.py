from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):
    initial = True
    dependencies = [
        ("clients", "0009_devicecommand"),
        ("tenants", "0001_initial"),
    ]

    operations = [
        migrations.CreateModel(
            name="Firmware",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False)),
                ("device_type", models.CharField(max_length=20, choices=[
                    ("shelly_em", "Shelly EM"), ("nous_at", "Nous AT"),
                    ("zigbee_sensor", "Zigbee Sensor"), ("auto_detected", "Auto Detected"),
                ])),
                ("version", models.CharField(max_length=50)),
                ("file_url", models.CharField(max_length=500)),
                ("checksum_sha256", models.CharField(max_length=64)),
                ("size_bytes", models.PositiveIntegerField(default=0)),
                ("release_notes", models.TextField(blank=True)),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                ("tenant", models.ForeignKey(
                    on_delete=django.db.models.deletion.PROTECT,
                    related_name="firmwares",
                    to="tenants.tenant",
                )),
                ("created_by", models.ForeignKey(
                    blank=True, null=True,
                    on_delete=django.db.models.deletion.SET_NULL,
                    to="clients.client",
                )),
            ],
            options={"unique_together": {("tenant", "device_type", "version")}},
        ),
        migrations.CreateModel(
            name="RolloutPlan",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False)),
                ("status", models.CharField(max_length=20, choices=[
                    ("pending", "Pending"), ("canary", "Canary"),
                    ("rolling", "Rolling"), ("complete", "Complete"),
                    ("rolled_back", "Rolled Back"), ("paused", "Paused"),
                ], default="pending")),
                ("canary_percent", models.PositiveSmallIntegerField(default=5)),
                ("current_percent", models.PositiveSmallIntegerField(default=0)),
                ("target_percent", models.PositiveSmallIntegerField(default=100)),
                ("step_percent", models.PositiveSmallIntegerField(default=10)),
                ("error_threshold", models.FloatField(default=0.1)),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                ("started_at", models.DateTimeField(blank=True, null=True)),
                ("completed_at", models.DateTimeField(blank=True, null=True)),
                ("firmware", models.OneToOneField(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="rollout",
                    to="ota.firmware",
                )),
                ("tenant", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="rollouts",
                    to="tenants.tenant",
                )),
            ],
        ),
        migrations.CreateModel(
            name="DeviceOTAStatus",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False)),
                ("status", models.CharField(max_length=20, choices=[
                    ("pending", "Pending"), ("sent", "Sent"),
                    ("downloading", "Downloading"), ("installing", "Installing"),
                    ("success", "Success"), ("failed", "Failed"),
                ], default="pending")),
                ("error_message", models.TextField(blank=True)),
                ("sent_at", models.DateTimeField(blank=True, null=True)),
                ("updated_at", models.DateTimeField(auto_now=True)),
                ("device", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="ota_statuses",
                    to="clients.device",
                )),
                ("firmware", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="device_statuses",
                    to="ota.firmware",
                )),
                ("rollout", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="device_statuses",
                    to="ota.rolloutplan",
                )),
            ],
            options={"unique_together": {("device", "firmware")}},
        ),
    ]
