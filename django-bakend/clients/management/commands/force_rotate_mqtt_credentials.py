"""Force-rotate MQTT credentials pentru device-urile fără mqtt_password_hash.

Folosit la migrarea de la "compat mode allow fără hash" → "obligatoriu hash"
(security hardening). Generează parole noi, salvează hash-urile în DB și
afișează parolele plain o singură dată — operatorul trebuie să le flush-uiască
în firmware-uri.

Usage:
    # Vezi câte device-uri n-au hash, fără să le modifici:
    python manage.py force_rotate_mqtt_credentials --dry-run

    # Rotește un singur device:
    python manage.py force_rotate_mqtt_credentials --device SHELF001

    # Rotește toate device-urile fără hash (BIG BANG):
    python manage.py force_rotate_mqtt_credentials --all

    # Rotește toate, inclusiv pe cele care AU deja hash (force-reset):
    python manage.py force_rotate_mqtt_credentials --all --include-existing
"""
import csv
import secrets
import sys

from django.contrib.auth.hashers import make_password
from django.core.management.base import BaseCommand, CommandError

from clients.models import Device


class Command(BaseCommand):
    help = "Force-rotate MQTT password pentru device-urile fără mqtt_password_hash"

    def add_arguments(self, parser):
        parser.add_argument(
            "--dry-run",
            action="store_true",
            help="Doar listează device-urile fără hash, fără modificări.",
        )
        parser.add_argument(
            "--device",
            help="Rotește doar device-ul cu acest serial_number.",
        )
        parser.add_argument(
            "--all",
            action="store_true",
            help="Rotește toate device-urile fără hash.",
        )
        parser.add_argument(
            "--include-existing",
            action="store_true",
            help="Cu --all, rotește și device-urile care AU deja hash.",
        )
        parser.add_argument(
            "--csv",
            help="Path fișier CSV pentru output (serial,password). Default stdout.",
        )

    def handle(self, *args, **opts):
        if not any([opts["dry_run"], opts["device"], opts["all"]]):
            raise CommandError("Specifică unul din --dry-run, --device <serial> sau --all.")

        qs = Device.objects.all()
        if opts["device"]:
            qs = qs.filter(serial_number=opts["device"])
            if not qs.exists():
                raise CommandError(f"Device cu serial {opts['device']} nu există.")
        elif not opts["include_existing"]:
            qs = qs.filter(mqtt_password_hash="")

        if opts["dry_run"]:
            self.stdout.write(self.style.NOTICE(f"Device-uri afectate: {qs.count()}"))
            for d in qs:
                state = "no-hash" if not d.mqtt_password_hash else "has-hash"
                self.stdout.write(f"  {d.serial_number}  tenant={d.tenant_id}  [{state}]")
            return

        total = qs.count()
        if total == 0:
            self.stdout.write(self.style.SUCCESS("Niciun device de rotat — toate au deja hash."))
            return

        out_csv = None
        if opts["csv"]:
            out_csv = csv.writer(open(opts["csv"], "w", newline="", encoding="utf-8"))
            out_csv.writerow(["serial_number", "mqtt_password", "tenant_id"])
        else:
            self.stdout.write("─" * 64)
            self.stdout.write(self.style.WARNING("PAROLE PLAIN — vizibile o singură dată"))
            self.stdout.write("─" * 64)

        rotated = 0
        for d in qs:
            plain = secrets.token_urlsafe(24)
            d.mqtt_password_hash = make_password(plain, hasher="bcrypt_sha256")
            d.save(update_fields=["mqtt_password_hash"])
            if out_csv:
                out_csv.writerow([d.serial_number, plain, d.tenant_id])
            else:
                self.stdout.write(f"{d.serial_number}  tenant={d.tenant_id}  password={plain}")
            rotated += 1

        if out_csv:
            self.stdout.write(self.style.SUCCESS(f"✓ Scris {rotated} rânduri în {opts['csv']}"))
            self.stdout.write(self.style.WARNING("Asigură-te că CSV-ul e chmod 600 + șterge după ce flush-uiești device-urile."))
        else:
            self.stdout.write("─" * 64)
            self.stdout.write(self.style.SUCCESS(f"✓ Rotated {rotated} device(s)"))
            self.stdout.write(self.style.WARNING("Salvează parolele ACUM — la următorul rotate, hash-ul se schimbă irevocabil."))
