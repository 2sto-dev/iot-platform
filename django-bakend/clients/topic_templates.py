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
    "zigbee_sensor": [
        "zigbee2mqtt/{serial}"
    ],
    "auto_detected": [
        "{serial}"
    ],
}
