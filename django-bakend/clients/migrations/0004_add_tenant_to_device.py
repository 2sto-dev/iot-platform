import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ("clients", "0003_auto_add_zigbee_autodetected"),
        ("tenants", "0001_initial"),
    ]

    operations = [
        migrations.AddField(
            model_name="device",
            name="tenant",
            field=models.ForeignKey(
                null=True,
                on_delete=django.db.models.deletion.PROTECT,
                related_name="devices",
                to="tenants.tenant",
            ),
        ),
    ]
