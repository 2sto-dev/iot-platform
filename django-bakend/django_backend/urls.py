from django.contrib import admin
from django.urls import path, include
from rest_framework_simplejwt.views import TokenRefreshView

# importăm view-ul custom care folosește serializerul nostru
from clients.views import CustomTokenObtainPairView

urlpatterns = [
    path("admin/", admin.site.urls),

    # include api-ul din aplicația clients cu namespace
    path("api/", include(("clients.urls", "clients"), namespace="clients")),

    # JWT (custom pentru username în token)
    path("api/token/", CustomTokenObtainPairView.as_view(), name="token_obtain_pair"),
    path("api/token/refresh/", TokenRefreshView.as_view(), name="token_refresh"),
]
