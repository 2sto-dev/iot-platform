from django.contrib import admin
from django.contrib.auth.admin import UserAdmin
from django.utils.html import format_html
from .models import Client, Device
from .topic_templates import TOPIC_TEMPLATES


@admin.register(Client)
class ClientAdmin(UserAdmin):
    """Admin pentru utilizatori (Client)"""
    model = Client
    list_display = ("username", "email", "prenume", "telefon", "is_staff", "is_superuser")
    fieldsets = UserAdmin.fieldsets + (
        ("Informații suplimentare", {"fields": ("prenume", "telefon")}),
    )
    add_fieldsets = UserAdmin.add_fieldsets + (
        ("Informații suplimentare", {"fields": ("prenume", "telefon")}),
    )


@admin.register(Device)
class DeviceAdmin(admin.ModelAdmin):
    list_display = ("serial_number", "description", "client", "device_type", "get_topics")
    readonly_fields = ("show_topics",)

    def get_topics(self, obj):
        topics = TOPIC_TEMPLATES.get(obj.device_type, [])
        topics = [t.replace("{serial}", obj.serial_number) for t in topics]
        return ", ".join(topics)
    get_topics.short_description = "MQTT Topics"

    def show_topics(self, obj):
        topics = TOPIC_TEMPLATES.get(obj.device_type, [])
        if not topics:
            return "—"
        topics = [t.replace("{serial}", obj.serial_number) for t in topics]
        html_list = "<ul style='list-style-type: disc; padding-left: 20px;'>"
        html_list += "".join([f"<li>{t}</li>" for t in topics])
        html_list += "</ul>"
        return format_html(html_list)
    show_topics.short_description = "MQTT Topics"

    def formfield_for_foreignkey(self, db_field, request, **kwargs):
        if db_field.name == "client":
            if not request.user.is_superuser:
                kwargs["queryset"] = Client.objects.filter(id=request.user.id)
                kwargs["initial"] = request.user.id
        return super().formfield_for_foreignkey(db_field, request, **kwargs)

    def save_model(self, request, obj, form, change):
        if not request.user.is_superuser:
            obj.client = request.user
        super().save_model(request, obj, form, change)

    def get_queryset(self, request):
        qs = super().get_queryset(request)
        if request.user.is_superuser:
            return qs
        return qs.filter(client=request.user)

    def has_module_permission(self, request):
        return request.user.is_staff or request.user.is_superuser
