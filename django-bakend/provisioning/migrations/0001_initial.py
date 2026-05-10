from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):

    initial = True

    dependencies = [
        ("clients", "0007_device_mqtt_password_hash"),
    ]

    operations = [
        migrations.CreateModel(
            name="ActivationToken",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("token_hash", models.CharField(max_length=64)),
                ("used", models.BooleanField(default=False)),
                ("expires_at", models.DateTimeField()),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                (
                    "device",
                    models.OneToOneField(
                        on_delete=django.db.models.deletion.CASCADE,
                        related_name="activation_token",
                        to="clients.device",
                    ),
                ),
            ],
        ),
    ]
