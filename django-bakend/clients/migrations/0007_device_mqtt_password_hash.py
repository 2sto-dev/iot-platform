from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ("clients", "0006_finalize_tenant_constraints"),
    ]

    operations = [
        migrations.AddField(
            model_name="device",
            name="mqtt_password_hash",
            field=models.CharField(blank=True, max_length=128),
        ),
    ]
