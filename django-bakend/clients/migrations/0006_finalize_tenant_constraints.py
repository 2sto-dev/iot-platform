import django.db.models.deletion
from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ("clients", "0005_populate_legacy_tenant"),
    ]

    operations = [
        migrations.AlterField(
            model_name="device",
            name="tenant",
            field=models.ForeignKey(
                on_delete=django.db.models.deletion.PROTECT,
                related_name="devices",
                to="tenants.tenant",
            ),
        ),
        migrations.AlterField(
            model_name="device",
            name="serial_number",
            field=models.CharField(max_length=100),
        ),
        migrations.AlterUniqueTogether(
            name="device",
            unique_together={("tenant", "serial_number")},
        ),
    ]
