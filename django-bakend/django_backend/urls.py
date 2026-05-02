from django.contrib import admin
from django.urls import path, include
from rest_framework_simplejwt.views import TokenRefreshView
from clients.views import CustomTokenObtainPairView
from tenants.mqtt_views import MQTTAuthView, MQTTACLView

urlpatterns = [
    path("admin/", admin.site.urls),

    # API clients (devices + user_devices)
    path("api/", include(("clients.urls", "clients"), namespace="clients")),

    # JWT login + refresh (emitere tokenuri de către Django)
    path("api/token/", CustomTokenObtainPairView.as_view(), name="token_obtain_pair"),
    path("api/token/refresh/", TokenRefreshView.as_view(), name="token_refresh"),

    # MQTT HTTP Auth/ACL hook — apelat de EMQX pe connect/publish/subscribe (Faza 2.1)
    path("api/mqtt/auth/", MQTTAuthView.as_view(), name="mqtt_auth"),
    path("api/mqtt/acl/", MQTTACLView.as_view(), name="mqtt_acl"),
]
