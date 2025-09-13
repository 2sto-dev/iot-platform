from django.contrib import admin
from django.urls import path, include
from rest_framework_simplejwt.views import TokenRefreshView
from clients.views import CustomTokenObtainPairView

urlpatterns = [
    path("admin/", admin.site.urls),

    # API clients (devices + user_devices)
    path("api/", include(("clients.urls", "clients"), namespace="clients")),

    # JWT login + refresh (emitere tokenuri de către Django)
    path("api/token/", CustomTokenObtainPairView.as_view(), name="token_obtain_pair"),
    path("api/token/refresh/", TokenRefreshView.as_view(), name="token_refresh"),
]
