# clients/topic_templates.py

# Dicționar cu template-uri de topicuri pentru fiecare tip de device.
# {serial} va fi înlocuit cu serial_number din modelul Device.

TOPIC_TEMPLATES = {
    "shelly_em": [
        "shellies/{serial}/emeter/0/energy",
        "shellies/{serial}/emeter/0/voltage",
        "shellies/{serial}/emeter/0/power",
        "shellies/{serial}/emeter/0/total",
    ],
    "nous_at": [
        "tele/{serial}/STATE",
        "tele/{serial}/SENSOR",
    ],
}
