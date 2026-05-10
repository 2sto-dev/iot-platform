package parsers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-iot-platform/internal/registry"
)

// parseGeneric — fallback când streamID nu match niciun parser specializat.
//
// Strategie:
//  1. Încercăm JSON unmarshal flat → toate field-urile top-level scrise as-is
//  2. Dacă nu e JSON valid, încercăm float64 (single value)
//  3. Dacă nu e nici număr, scriem ca string sub field-ul "value"
//
// Output Influx:
//
//	source=generic type=auto_detected fields={...top-level keys sau "value": x}
//
// Acest parser asigură că device-uri necunoscute / topice noi nu sunt drop-uite
// silentios — măcar ajung în Influx pt audit / debugging.
func parseGeneric(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	// 1. Try JSON object
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err == nil && len(data) > 0 {
		return &ParsedTelemetry{
			Timestamp: time.Now(),
			Source:    "generic",
			Type:      "auto_detected",
			Fields:    data,
		}, nil
	}

	// 2. Try plain number
	valStr := strings.TrimSpace(string(payload))
	if f, err := strconv.ParseFloat(valStr, 64); err == nil {
		return &ParsedTelemetry{
			Timestamp: time.Now(),
			Source:    "generic",
			Type:      "auto_detected",
			Fields:    map[string]interface{}{"value": f},
		}, nil
	}

	// 3. Fall back to string field
	if valStr == "" {
		return nil, fmt.Errorf("%w: empty generic payload", errMalformedPayload)
	}
	return &ParsedTelemetry{
		Timestamp: time.Now(),
		Source:    "generic",
		Type:      "auto_detected",
		Fields:    map[string]interface{}{"value": valStr},
	}, nil
}
