package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"go-iot-platform/internal/registry"
)

// parseZigbeeJSON — pentru topic-uri zigbee2mqtt/<friendly_name> sau .../up/zigbee.
//
// Payload e flat JSON cu chei standard Z2M:
//
//	{"temperature":21.5,"humidity":56,"battery":85,"linkquality":42,"voltage":3000}
//
// Output Influx:
//
//	source=zigbee2mqtt type=sensor fields={...all top-level keys...}
//
// Toate field-urile top-level sunt scrise as-is, ceea ce permite Z2M să adauge
// fields noi (ex: pressure, motion, illuminance) fără să cere parser update.
func parseZigbeeJSON(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("zigbee json unmarshal: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty zigbee payload", errMalformedPayload)
	}
	return &ParsedTelemetry{
		Timestamp: time.Now(),
		Source:    "zigbee2mqtt",
		Type:      "sensor",
		Fields:    data,
	}, nil
}
