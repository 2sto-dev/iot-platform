"""Create or update the iot-ingest service user used by the Go ingest service.

Idempotent: re-running just ensures the user exists, has the required permissions,
and (optionally) updates the password.
"""
import os

from django.contrib.auth import get_user_model
from django.contrib.auth.models import Permission
from django.contrib.contenttypes.models import ContentType
from django.core.management.base import BaseCommand

from clients.models import Device


SERVICE_USERNAME_DEFAULT = "iot-ingest"
REQUIRED_PERMS = ("view_device", "add_device")


class Command(BaseCommand):
    help = "Ensure the iot-ingest service user exists with minimal device permissions."

    def add_arguments(self, parser):
        parser.add_argument("--username", default=os.getenv("DJANGO_SERVICE_USER", SERVICE_USERNAME_DEFAULT))
        parser.add_argument("--password", default=os.getenv("DJANGO_SERVICE_PASS"))

    def handle(self, *args, **opts):
        username = opts["username"]
        password = opts["password"]
        if not password:
            self.stderr.write(self.style.ERROR(
                "Password required: pass --password or set DJANGO_SERVICE_PASS env var."
            ))
            return

        User = get_user_model()
        user, created = User.objects.get_or_create(
            username=username,
            defaults={"prenume": "IoT Ingest Service", "is_staff": False, "is_superuser": False},
        )
        user.set_password(password)
        user.is_active = True
        user.save()

        ct = ContentType.objects.get_for_model(Device)
        perms = list(Permission.objects.filter(content_type=ct, codename__in=REQUIRED_PERMS))
        if len(perms) != len(REQUIRED_PERMS):
            missing = set(REQUIRED_PERMS) - {p.codename for p in perms}
            self.stderr.write(self.style.ERROR(f"Missing permissions: {missing}. Run migrate first."))
            return
        user.user_permissions.set(perms)

        action = "Created" if created else "Updated"
        self.stdout.write(self.style.SUCCESS(
            f"{action} service user '{username}' with permissions: {', '.join(REQUIRED_PERMS)}"
        ))
