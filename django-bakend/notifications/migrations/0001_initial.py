from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):
    initial = True
    dependencies = [
        ("tenants", "0001_initial"),
    ]

    operations = [
        migrations.CreateModel(
            name="NotificationChannel",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("name", models.CharField(max_length=100)),
                ("type", models.CharField(
                    choices=[("webhook", "Webhook HTTP"), ("email", "Email"), ("fcm", "Firebase Cloud Messaging")],
                    max_length=20,
                )),
                ("config", models.JSONField()),
                ("enabled", models.BooleanField(default=True)),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                ("tenant", models.ForeignKey(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="notification_channels",
                    to="tenants.tenant",
                )),
            ],
            options={"ordering": ["name"], "unique_together": {("tenant", "name")}},
        ),
        migrations.CreateModel(
            name="NotificationEvent",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("channel_name", models.CharField(max_length=100)),
                ("rule_execution_id", models.BigIntegerField(blank=True, null=True)),
                ("title", models.CharField(blank=True, max_length=200)),
                ("body", models.TextField()),
                ("context", models.JSONField(default=dict)),
                ("status", models.CharField(
                    choices=[("pending", "Pending"), ("sent", "Sent"), ("failed", "Failed")],
                    default="pending",
                    max_length=20,
                )),
                ("attempts", models.PositiveIntegerField(default=0)),
                ("last_error", models.TextField(blank=True)),
                ("created_at", models.DateTimeField(auto_now_add=True, db_index=True)),
                ("sent_at", models.DateTimeField(blank=True, null=True)),
                ("channel", models.ForeignKey(
                    blank=True,
                    null=True,
                    on_delete=django.db.models.deletion.SET_NULL,
                    related_name="events",
                    to="notifications.notificationchannel",
                )),
            ],
            options={"ordering": ["-created_at"]},
        ),
    ]
