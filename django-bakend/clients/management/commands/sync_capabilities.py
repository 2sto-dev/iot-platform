"""Faza 5: re-populeaza Device.capabilities din DD YAML files pentru toate device-urile.

Folosit:
  - La migration initial pentru backfill device-urile existente
  - Dupa modificare YAML configs (add/remove capabilities) — re-sync producție
  - In CI/test ca smoke check ca toate device_type-urile au mapping în dd_loader

Usage:
    python manage.py sync_capabilities                # toate tenantii
    python manage.py sync_capabilities --tenant 2     # doar tenant id=2
    python manage.py sync_capabilities --device 39371381  # un singur device
    python manage.py sync_capabilities --dry-run      # arata ce s-ar schimba
    python manage.py sync_capabilities --force        # suprascrie capabilities setate manual
"""
from django.core.management.base import BaseCommand

from clients.dd_loader import get_capabilities_for_device_type, reload_dd_cache
from clients.models import Device


class Command(BaseCommand):
    help = "Sincronizeaza Device.capabilities din configs/devices/*.yaml"

    def add_arguments(self, parser):
        parser.add_argument("--tenant", type=int, help="Tenant id (default: toate)")
        parser.add_argument("--device", help="Serial number (default: toate)")
        parser.add_argument("--dry-run", action="store_true", help="Arata schimbarile fara save")
        parser.add_argument("--force", action="store_true",
                            help="Suprascrie capabilities setate manual (default: skip non-empty)")

    def handle(self, *args, **opts):
        reload_dd_cache()  # asigura că modificările YAML sunt vizibile

        qs = Device.objects.all().select_related("tenant")
        if opts["tenant"]:
            qs = qs.filter(tenant_id=opts["tenant"])
        if opts["device"]:
            qs = qs.filter(serial_number=opts["device"])

        total = qs.count()
        updated = 0
        skipped_existing = 0
        skipped_no_dd = 0

        for d in qs:
            target = get_capabilities_for_device_type(d.device_type)
            if not target:
                self.stdout.write(self.style.WARNING(
                    f"  - {d.serial_number} ({d.device_type}): no DD mapping, skip"
                ))
                skipped_no_dd += 1
                continue

            current = d.capabilities or []
            if current and not opts["force"]:
                if set(current) == set(target):
                    # Already in sync, no message
                    pass
                else:
                    self.stdout.write(self.style.WARNING(
                        f"  - {d.serial_number}: existing capabilities differ "
                        f"(current={current}, dd={target}); use --force to override"
                    ))
                    skipped_existing += 1
                continue

            if opts["dry_run"]:
                self.stdout.write(
                    f"  [DRY] {d.serial_number} ({d.device_type}): "
                    f"{current} -> {target}"
                )
            else:
                d.capabilities = target
                d.save(update_fields=["capabilities"])
                self.stdout.write(self.style.SUCCESS(
                    f"  + {d.serial_number} ({d.device_type}): {target}"
                ))
            updated += 1

        self.stdout.write("")
        self.stdout.write(self.style.SUCCESS(
            f"Done: {updated}/{total} updated"
            + (f", {skipped_existing} skipped (--force pentru override)" if skipped_existing else "")
            + (f", {skipped_no_dd} fara DD" if skipped_no_dd else "")
            + (" [DRY-RUN]" if opts["dry_run"] else "")
        ))
