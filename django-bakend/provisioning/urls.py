from django.urls import path
from .views import RotateCredentialView, MQTTAuthView

app_name = "provisioning"

urlpatterns = [
    path("devices/<int:device_id>/credentials/rotate/", RotateCredentialView.as_view(), name="rotate"),
    path("mqtt-auth/", MQTTAuthView.as_view(), name="mqtt_auth"),
]
