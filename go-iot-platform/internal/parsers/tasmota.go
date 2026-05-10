package parsers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-iot-platform/internal/registry"
)

// stateMessage — payload Tasmota tele/<topic>/STATE.
//
// Format minim observat:
//
//	{"POWER":"ON","RSSI":42,"Wifi":{"SSId":"X","RSSI":-65},"Heap":24,"LoadAvg":19,...}
//
// Nu interesează decât POWER (relay state) și RSSI.
type stateMessage struct {
	POWER string `json:"POWER"`
	RSSI  int    `json:"RSSI"`
}

// parseTasmotaState — pentru topic-uri tele/<id>/STATE sau .../up/state.
//
// Output Influx:
//
//	source=nousat type=state fields={relay_state, relay_on, rssi}
//
// `relay_state` e string ("ON"/"OFF") pentru audit.
// `relay_on` e int 1/0 pentru polling friendly UI confirm.
// `rssi` e dBm raportat de Tasmota.
func parseTasmotaState(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	var m stateMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("tasmota state unmarshal: %w", err)
	}
	relayOn := 0
	if strings.EqualFold(m.POWER, "ON") {
		relayOn = 1
	}
	return &ParsedTelemetry{
		Timestamp: time.Now(),
		Source:    "nousat",
		Type:      "state",
		Fields: map[string]interface{}{
			"relay_state": m.POWER,
			"relay_on":    relayOn,
			"rssi":        m.RSSI,
		},
	}, nil
}

// energyData — sub-object ENERGY din Tasmota tele/<topic>/SENSOR.
type energyData struct {
	Total         float64 `json:"Total"`
	Today         float64 `json:"Today"`
	Yesterday     float64 `json:"Yesterday"`
	Power         float64 `json:"Power"`
	ApparentPower float64 `json:"ApparentPower"`
	ReactivePower float64 `json:"ReactivePower"`
	Factor        float64 `json:"Factor"`
	Voltage       float64 `json:"Voltage"`
	Current       float64 `json:"Current"`
}

type sensorMessage struct {
	Time   string     `json:"Time"`
	ENERGY energyData `json:"ENERGY"`
}

// parseTasmotaSensor — pentru topic-uri tele/<id>/SENSOR sau .../up/sensor.
//
// Output Influx:
//
//	source=nousat type=energy fields={nousat_power, nousat_voltage, ...}
//
// Field-urile au prefix `nousat_` ca să eviti type conflicts pe scopul
// "devices" measurement (vezi notes ADR-001 + main.go: `power` field a fost
// cuplat cu string ON/OFF de un STATE handler bugged — fix-ul rename
// preserves Influx schema healthy).
func parseTasmotaSensor(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	var m sensorMessage
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("tasmota sensor unmarshal: %w", err)
	}
	t := time.Now()
	if m.Time != "" {
		if pt, err := time.Parse(time.RFC3339, m.Time); err == nil {
			t = pt
		}
	}
	return &ParsedTelemetry{
		Timestamp: t,
		Source:    "nousat",
		Type:      "energy",
		Fields: map[string]interface{}{
			"nousat_power":          m.ENERGY.Power,
			"nousat_apparent_power": m.ENERGY.ApparentPower,
			"nousat_reactive_power": m.ENERGY.ReactivePower,
			"nousat_power_factor":   m.ENERGY.Factor,
			"nousat_voltage":        m.ENERGY.Voltage,
			"nousat_current":        m.ENERGY.Current,
			"nousat_total":          m.ENERGY.Total,
			"nousat_today":          m.ENERGY.Today,
			"nousat_yesterday":      m.ENERGY.Yesterday,
		},
	}, nil
}
