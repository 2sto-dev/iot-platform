from rest_framework import serializers
from .models import Device
from .topic_templates import TOPIC_TEMPLATES

class DeviceSerializer(serializers.ModelSerializer):
    topics = serializers.SerializerMethodField()

    class Meta:
        model = Device
        fields = ["id", "serial_number", "description", "device_type", "topics"]

    def get_topics(self, obj):
        """Generează topicurile pentru device din template-uri."""
        template_list = TOPIC_TEMPLATES.get(obj.device_type, [])
        return [t.format(serial=obj.serial_number) for t in template_list]



