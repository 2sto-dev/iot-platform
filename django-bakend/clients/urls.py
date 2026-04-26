from django.urls import path, include
from rest_framework.routers import DefaultRouter
from .views import DeviceViewSet

app_name = "clients"

router = DefaultRouter()
router.register(r'devices', DeviceViewSet, basename='device')

urlpatterns = [
    path('', include(router.urls)),
]
