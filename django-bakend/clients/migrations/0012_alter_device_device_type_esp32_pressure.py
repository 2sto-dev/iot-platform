# Faza follow-up: adaug 4 device types ESP32 cu senzori de presiune.

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ("clients", "0011_device_capabilities"),
    ]

    operations = [
        migrations.AlterField(
            model_name="device",
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
