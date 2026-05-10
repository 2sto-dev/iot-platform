# Sync OTA Firmware.device_type cu noile choices din clients.Device.

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ("ota", "0002_alter_deviceotastatus_id_alter_firmware_device_type_and_more"),
        ("clients", "0012_alter_device_device_type_esp32_pressure"),
    ]

    operations = [
        migrations.AlterField(
            model_name="firmware",
            name="device_type",
            field=models.CharField(
                choices=[
                    ("shelly_em", "Shelly EM"),
                    ("nous_at", "Nous AT"),
                    ("zigbee_sensor", "Zigbee Sensor"),
                    ("auto_detected", "Auto Detected"),
                    ("sun2000", "Huawei SUN2000"),
                    ("esp32_bmp180", "ESP32 BMP180"),
                    ("esp32_bmp280", "ESP32 BMP280"),
                    ("esp32_bme280", "ESP32 BME280"),
                    ("esp32_ms5611", "ESP32 MS5611"),
                ],
                max_length=20,
            ),
        ),
    ]
