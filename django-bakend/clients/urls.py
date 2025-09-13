from django.urls import path, include
from rest_framework.routers import DefaultRouter
from .views import DeviceViewSet, user_devices

app_name = "clients"

router = DefaultRouter()
router.register(r'devices', DeviceViewSet, basename='device')

urlpatterns = [
    path('devices/<str:username>/', user_devices, name='user_devices'),
    path('', include(router.urls)),
]
