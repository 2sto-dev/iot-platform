from django.contrib import admin
from django.urls import path, include
from rest_framework_simplejwt.views import TokenRefreshView
from drf_spectacular.views import SpectacularAPIView, SpectacularSwaggerView, SpectacularRedocView
from clients.views import CustomTokenObtainPairView
from tenants.mqtt_views import MQTTAuthView, MQTTACLView
from rules.urls import internal_urlpatterns as rules_internal
from notifications.urls import internal_urlpatterns as notif_internal

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

    # Provisioning — activation flow (Faza 3.2)
    path("api/provisioning/", include("provisioning.urls")),

    # OTA service — firmware + staged rollout (Faza 3.5)
    path("api/ota/", include("ota.urls")),

    # v1 versioned endpoints (Faza 4)
    path("api/v1/audit/", include("audit.urls")),
    path("api/v1/api-keys/", include("api_keys.urls")),
    path("api/v1/rules/", include("rules.urls")),
    path("api/v1/notifications/", include("notifications.urls")),

    # Internal endpoints — service accounts / Go workers only
    path("api/internal/", include(rules_internal)),
    path("api/internal/", include(notif_internal)),

    # OpenAPI schema + UI (drf-spectacular)
    path("api/schema/", SpectacularAPIView.as_view(), name="schema"),
    path("api/docs/", SpectacularSwaggerView.as_view(url_name="schema"), name="swagger-ui"),
    path("api/redoc/", SpectacularRedocView.as_view(url_name="schema"), name="redoc"),
]
