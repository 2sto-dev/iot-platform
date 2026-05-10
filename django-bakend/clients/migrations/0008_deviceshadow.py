from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):
    dependencies = [("clients", "0007_device_mqtt_password_hash")]

    operations = [
        migrations.CreateModel(
            name="DeviceShadow",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("reported", models.JSONField(default=dict)),
                ("desired", models.JSONField(default=dict)),
                ("version", models.PositiveIntegerField(default=0)),
                ("updated_at", models.DateTimeField(auto_now=True)),
                ("device", models.OneToOneField(
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="shadow",
                    to="clients.device",
                )),
            ],
        ),
    ]
