from django.urls import path
from .views import ActivateView

urlpatterns = [
    path("activate/", ActivateView.as_view(), name="provisioning_activate"),
]
