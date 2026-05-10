"""Management command pentru avansarea automată a rollout-urilor active.

Poate fi apelat de cron sau manual:
  python manage.py advance_rollout --all
  python manage.py advance_rollout --rollout-id 5
"""
from django.core.management.base import BaseCommand
from django.utils import timezone

from ota.models import DeviceOTAStatus, RolloutPlan
from ota.views import _dispatch_batch, _eligible_devices

import random


class Command(BaseCommand):
    help = "Avansează rollout-urile active la etapa următoare (canary → rolling → complete)."

    def add_arguments(self, parser):
        group = parser.add_mutually_exclusive_group(required=True)
        group.add_argument("--all", action="store_true", help="Avansează toate rollout-urile active.")
        group.add_argument("--rollout-id", type=int, help="ID-ul rollout-ului de avansat.")

    def handle(self, *args, **options):
        if options["all"]:
            rollouts = RolloutPlan.objects.filter(
                status__in=[RolloutPlan.Status.CANARY, RolloutPlan.Status.ROLLING]
            )
        else:
            rollouts = RolloutPlan.objects.filter(pk=options["rollout_id"])

        for rollout in rollouts:
            self._advance(rollout)

    def _advance(self, rollout):
        if rollout.should_auto_rollback():
            rollout.status = RolloutPlan.Status.ROLLED_BACK
            rollout.completed_at = timezone.now()
            rollout.save(update_fields=["status", "completed_at"])
            self.stdout.write(self.style.WARNING(
                f"Rollout {rollout.id} auto-rolled back (error rate {rollout.error_rate:.0%})"
            ))
            return

        next_percent = min(rollout.current_percent + rollout.step_percent, rollout.target_percent)
        eligible = _eligible_devices(rollout)
        n_next = max(0, int(len(eligible) * (next_percent - rollout.current_percent) / 100))
        batch = random.sample(eligible, min(n_next, len(eligible)))
        _dispatch_batch(rollout, batch)

        rollout.current_percent = next_percent
        rollout.status = RolloutPlan.Status.COMPLETE if next_percent >= rollout.target_percent \
            else RolloutPlan.Status.ROLLING
        if rollout.status == RolloutPlan.Status.COMPLETE:
            rollout.completed_at = timezone.now()
        rollout.save(update_fields=["status", "current_percent", "completed_at"])
        self.stdout.write(self.style.SUCCESS(
            f"Rollout {rollout.id}: {rollout.current_percent}% → status={rollout.status}"
        ))
