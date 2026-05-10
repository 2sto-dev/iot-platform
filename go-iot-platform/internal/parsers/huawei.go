package parsers

import (
	"encoding/json"
	"fmt"
	"time"

	"go-iot-platform/internal/registry"
)

// huaweiTelemetry — payload SUN2000 telemetry.
//
// Format:
//
//	{
//	  "ts": "2026-05-10T12:34:56Z",
//	  "measurements": [
//	    {"key": "pv_input_power", "value": 8.66},
//	    {"key": "battery_soc", "value": 99},
//	    ...
//	  ],
//	  "house_load_kw_est": 0.51   // optional, computed by collector
//	}
type huaweiTelemetry struct {
	Ts           string                   `json:"ts"`
	Measurements []map[string]interface{} `json:"measurements"`
	HouseLoad    float64                  `json:"house_load_kw_est"`
}

// parseHuaweiTelemetry — pentru topic-uri Huawei SUN2000 (legacy /sn/coll/.../telemetry
// sau platform-native tenants/+/devices/+/up/telemetry).
//
// Output Influx:
//
//	source=sun2000 type=solar_inverter fields={pv_input_power, battery_soc, grid_power, ...}
//
// Toate field-urile din `measurements[].key` se preserv ca-and-as (vendor names),
// plus `house_load_kw_est` ca top-level field dacă e prezent și ≠ 0.
func parseHuaweiTelemetry(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	var msg huaweiTelemetry
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("huawei telemetry unmarshal: %w", err)
	}
	if len(msg.Measurements) == 0 && msg.HouseLoad == 0 {
		// Empty payload — nu e fatal, doar nu scriem nimic.
		return nil, fmt.Errorf("%w: empty measurements + house_load=0", errMalformedPayload)
	}

	fields := make(map[string]interface{}, len(msg.Measurements)+1)
	for _, m := range msg.Measurements {
		key, kok := m["key"].(string)
		if !kok || key == "" {
			continue
		}
		val, vok := m["value"]
		if !vok {
			continue
		}
		fields[key] = val
	}
	if msg.HouseLoad != 0 {
		fields["house_load_kw_est"] = msg.HouseLoad
	}

	t := time.Now()
	if msg.Ts != "" {
		if pt, err := time.Parse(time.RFC3339, msg.Ts); err == nil {
			t = pt
		}
	}

	return &ParsedTelemetry{
		Timestamp: t,
		Source:    "sun2000",
		Type:      "solar_inverter",
		Fields:    fields,
	}, nil
}
