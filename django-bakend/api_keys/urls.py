from django.urls import path
from .views import APIKeyListCreateView, APIKeyRevokeView

urlpatterns = [
    path("", APIKeyListCreateView.as_view(), name="apikey-list-create"),
    path("<int:pk>/", APIKeyRevokeView.as_view(), name="apikey-revoke"),
]
