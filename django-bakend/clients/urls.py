from django.urls import path, include
from rest_framework.routers import DefaultRouter
from .views import (
    DeviceViewSet,
    DeviceShadowView,
    DeviceShadowReportedView,
    DeviceShadowReportedBySerialView,
    DeviceCommandListCreateView,
    DeviceCommandDetailView,
    DeviceCommandAckView,
    TenantListView,
)
from ota.views import DeviceOTAHistoryView

app_name = "clients"

router = DefaultRouter()
router.register(r'devices', DeviceViewSet, basename='device')

urlpatterns = [
    path('', include(router.urls)),
    path('auth/tenants/', TenantListView.as_view(), name='auth-tenants'),
    path('devices/<int:pk>/shadow/', DeviceShadowView.as_view(), name='device-shadow'),
    path('devices/<int:pk>/shadow/reported/', DeviceShadowReportedView.as_view(), name='device-shadow-reported'),
    path('devices/<int:pk>/commands/', DeviceCommandListCreateView.as_view(), name='device-commands'),
    path('devices/<int:pk>/commands/<int:cmd_id>/', DeviceCommandDetailView.as_view(), name='device-command-detail'),
    path('devices/<int:pk>/commands/<int:cmd_id>/ack/', DeviceCommandAckView.as_view(), name='device-command-ack'),
    path('shadow/reported/', DeviceShadowReportedBySerialView.as_view(), name='shadow-reported-by-serial'),
    # Global command ACK — callers know cmd_id only (Go downlink-worker, MQTT ACK handler).
    path('devices/commands/<int:cmd_id>/ack/', DeviceCommandAckView.as_view(), name='command-ack-global'),
    path('devices/<int:pk>/ota/', DeviceOTAHistoryView.as_view(), name='device-ota-history'),
]
