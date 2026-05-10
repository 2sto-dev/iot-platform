"""Management command pentru generarea unui activation token per device.

Usage:
    python manage.py generate_activation_token --serial SHELF001
    python manage.py generate_activation_token --serial SHELF001 --expires-hours 72
"""
import hashlib
import secrets
from datetime import timedelta

from django.core.management.base import BaseCommand, CommandError
from django.utils import timezone

from clients.models import Device
from provisioning.models import ActivationToken


class Command(BaseCommand):
    help = "Generează un activation token one-time pentru un device."

    def add_arguments(self, parser):
        parser.add_argument("--serial", required=True, help="Serial number al device-ului")
        parser.add_argument("--expires-hours", type=int, default=72, help="Valabilitate în ore (default 72)")

    def handle(self, *args, **options):
        serial = options["serial"]
        hours = options["expires_hours"]

        try:
            device = Device.objects.get(serial_number=serial)
        except Device.DoesNotExist:
            raise CommandError(f"Device cu serial '{serial}' nu există.")

        token_plain = secrets.token_urlsafe(32)
        token_hash = hashlib.sha256(token_plain.encode()).hexdigest()
        expires_at = timezone.now() + timedelta(hours=hours)

        ActivationToken.objects.update_or_create(
            device=device,
            defaults={"token_hash": token_hash, "used": False, "expires_at": expires_at},
        )

        self.stdout.write(self.style.SUCCESS(f"Activation token generat pentru {serial}:"))
        self.stdout.write(f"  Token    : {token_plain}")
        self.stdout.write(f"  Expiră   : {expires_at.strftime('%Y-%m-%d %H:%M UTC')}")
        self.stdout.write(self.style.WARNING("  ⚠️  Salvează token-ul — nu mai poate fi recuperat!"))
