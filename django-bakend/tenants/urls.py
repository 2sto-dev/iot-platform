from django.urls import path
from .views import MQTTACLView, TenantPlanView

app_name = "tenants"

urlpatterns = [
    path("mqtt-acl/", MQTTACLView.as_view(), name="mqtt_acl"),
    path("tenants/<int:tenant_id>/plan/", TenantPlanView.as_view(), name="tenant_plan"),
]
