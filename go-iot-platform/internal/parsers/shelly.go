package parsers

import (
	"fmt"
	"strings"
	"time"

	"go-iot-platform/internal/registry"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// titleCaser — folosit la field naming în Shelly emeter (Power, Voltage, ...
// rămân cu inițială mare ca în versiunea pre-Faza-4 pentru back-compat dashboard).
var titleCaser = cases.Title(language.Und)

// parseShellyEmeter — pentru topic-uri shellies/+/emeter/0/<field>.
//
// Payload-ul Shelly e plain string per topic (ex: topic .../power → payload "1234.56"),
// deci field-name vine din ULTIMUL segment al topicului.
//
// Output Influx:
//
//	source=shelly type=power_meter fields={Power: 1234.56} (un single field per call)
//
// Notă: TitleCase pe field-name preserve back-compat (dashboard query "Power").
func parseShellyEmeter(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	valStr := string(payload)
	var value float64
	if _, err := fmt.Sscanf(valStr, "%f", &value); err != nil {
		return nil, fmt.Errorf("shelly emeter parse %q as float: %w", valStr, err)
	}
	parts := strings.Split(topic, "/")
	if len(parts) == 0 {
		return nil, fmt.Errorf("shelly emeter: empty topic")
	}
	field := parts[len(parts)-1]
	return &ParsedTelemetry{
		Timestamp: time.Now(),
		Source:    "shelly",
		Type:      "power_meter",
		Fields: map[string]interface{}{
			titleCaser.String(field): value,
		},
	}, nil
}

// parseShellyRelay — pentru topic-uri shellies/+/relay/0.
//
// Payload e plain string "on" sau "off". Convertim la int 1/0 pentru
// query-uri numerice friendly în Grafana / Influx.
//
// Output Influx:
//
//	source=shelly type=relay fields={state: 0|1}
func parseShellyRelay(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	valStr := strings.ToLower(string(payload))
	state := 0
	if valStr == "on" {
		state = 1
	}
	return &ParsedTelemetry{
		Timestamp: time.Now(),
		Source:    "shelly",
		Type:      "relay",
		Fields: map[string]interface{}{
			"state": state,
		},
	}, nil
}
