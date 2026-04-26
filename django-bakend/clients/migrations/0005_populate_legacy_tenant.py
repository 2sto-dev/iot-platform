from django.db import migrations


LEGACY_SLUG = "legacy"
LEGACY_NAME = "Legacy Tenant"


def populate_legacy_tenant(apps, schema_editor):
    Tenant = apps.get_model("tenants", "Tenant")
    Membership = apps.get_model("tenants", "Membership")
    Device = apps.get_model("clients", "Device")
    Client = apps.get_model("clients", "Client")

    legacy, _ = Tenant.objects.get_or_create(
        slug=LEGACY_SLUG,
        defaults={"name": LEGACY_NAME, "plan": "free", "status": "active"},
    )

    for user in Client.objects.all():
        Membership.objects.get_or_create(
            user=user,
            tenant=legacy,
            defaults={"role": "OWNER"},
        )

    Device.objects.filter(tenant__isnull=True).update(tenant=legacy)


def reverse_populate(apps, schema_editor):
    Device = apps.get_model("clients", "Device")
    Device.objects.update(tenant=None)


class Migration(migrations.Migration):

    dependencies = [
        ("clients", "0004_add_tenant_to_device"),
    ]

    operations = [
        migrations.RunPython(populate_legacy_tenant, reverse_populate),
    ]
