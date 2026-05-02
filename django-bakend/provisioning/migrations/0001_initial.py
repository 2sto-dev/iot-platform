from django.db import migrations, models
import django.db.models.deletion
import django.utils.timezone


class Migration(migrations.Migration):
    initial = True

    dependencies = [
        ("clients", "0006_finalize_tenant_constraints"),
    ]

    operations = [
        migrations.CreateModel(
            name="DeviceCredential",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("secret_hash", models.CharField(max_length=256)),
                ("status", models.CharField(choices=[("active", "Active"), ("revoked", "Revoked")], default="active", max_length=20)),
                ("rotated_at", models.DateTimeField(default=django.utils.timezone.now)),
                ("created_at", models.DateTimeField(auto_now_add=True)),
                ("updated_at", models.DateTimeField(auto_now=True)),
                ("device", models.OneToOneField(on_delete=django.db.models.deletion.CASCADE, related_name="credential", to="clients.device")),
            ],
        ),
    ]
