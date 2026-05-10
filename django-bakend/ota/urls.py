from django.urls import path
from .views import (
    FirmwareListCreateView,
    FirmwareDetailView,
    RolloutListCreateView,
    RolloutDetailView,
    RolloutAdvanceView,
    RolloutPauseView,
    RolloutRollbackView,
    DeviceOTAStatusUpdateView,
)

urlpatterns = [
    path("firmware/", FirmwareListCreateView.as_view(), name="ota-firmware-list"),
    path("firmware/<int:pk>/", FirmwareDetailView.as_view(), name="ota-firmware-detail"),
    path("rollouts/", RolloutListCreateView.as_view(), name="ota-rollout-list"),
    path("rollouts/<int:pk>/", RolloutDetailView.as_view(), name="ota-rollout-detail"),
    path("rollouts/<int:pk>/advance/", RolloutAdvanceView.as_view(), name="ota-rollout-advance"),
    path("rollouts/<int:pk>/pause/", RolloutPauseView.as_view(), name="ota-rollout-pause"),
    path("rollouts/<int:pk>/rollback/", RolloutRollbackView.as_view(), name="ota-rollout-rollback"),
    path("devices/<str:serial>/status/", DeviceOTAStatusUpdateView.as_view(), name="ota-device-status"),
]
