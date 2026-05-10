from django.conf import settings
from django.db import migrations, models
import django.db.models.deletion


class Migration(migrations.Migration):
    initial = True
    dependencies = [
        ("tenants", "0001_initial"),
        migrations.swappable_dependency(settings.AUTH_USER_MODEL),
    ]

    operations = [
        migrations.CreateModel(
            name="AuditLog",
            fields=[
                ("id", models.BigAutoField(auto_created=True, primary_key=True, serialize=False, verbose_name="ID")),
                ("action", models.CharField(
                    choices=[("create", "Create"), ("update", "Update"), ("delete", "Delete")],
                    max_length=20,
                )),
                ("resource_type", models.CharField(max_length=50)),
                ("resource_id", models.CharField(blank=True, max_length=100)),
                ("metadata", models.JSONField(default=dict)),
                ("ip", models.GenericIPAddressField(blank=True, null=True)),
                ("ts", models.DateTimeField(auto_now_add=True, db_index=True)),
                ("tenant", models.ForeignKey(
                    blank=True,
                    null=True,
                    on_delete=django.db.models.deletion.CASCADE,
                    related_name="audit_logs",
                    to="tenants.tenant",
                )),
                ("actor", models.ForeignKey(
                    blank=True,
                    null=True,
                    on_delete=django.db.models.deletion.SET_NULL,
                    related_name="audit_logs",
                    to=settings.AUTH_USER_MODEL,
                )),
            ],
            options={"ordering": ["-ts"]},
        ),
    ]
