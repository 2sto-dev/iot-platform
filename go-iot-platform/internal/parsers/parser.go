// Package parsers transformă payload-uri MQTT vendor-specific în ParsedTelemetry
// uniform, gata pentru scriere Influx.
//
// Înlocuiește struct-urile hardcoded (StateMessage, SensorMessage, EnergyData)
// din cmd/main.go și logica inline de parsing.
//
// Routing-ul către parser-ul corect se face pe `streamID` (returnat de
// internal/matcher.Match()). Mapping streamID → parser e definit in initParsers().
//
// Vezi: docs/adr/ADR-003-parser-engine.md
package parsers

import (
	"fmt"
	"time"

	"go-iot-platform/internal/registry"
)

// ParsedTelemetry — output uniform pentru orice parser.
//
// Caller-ul (cmd/main.go) folosește toate câmpurile direct la NewPoint:
//
//	tags := map[string]string{
//	    "device":    deviceID,
//	    "source":    pt.Source,
//	    "type":      pt.Type,
//	    "tenant_id": tenantTag,
//	}
//	p := influxdb2.NewPoint("devices", tags, pt.Fields, pt.Timestamp)
type ParsedTelemetry struct {
	Timestamp time.Time              // când s-a generat datele (sau time.Now() fallback)
	Source    string                 // tag Influx — sun2000 / nousat / shelly / zigbee2mqtt / generic
	Type      string                 // tag Influx — solar_inverter / state / energy / power_meter / relay / sensor / auto_detected
	Fields    map[string]interface{} // datele propriu-zise (vendor-named pt back-compat)
}

// ParserFunc — semnătură pentru un parser de stream.
//
// Parametrii:
//   - topic: topicul MQTT original (necesar pt parser-uri ca Shelly emeter
//     care extrag field-name din topic)
//   - payload: byte raw din mesaj
//   - dd: DeviceDefinition matched de matcher (opțional, parser-ul îl ignoră
//     dacă nu îi trebuie metadata din DD)
//   - extracted: variabilele extrase de matcher din topic (device_id, tenant_id,
//     etc.) — utile pt parser-uri care leagă field-uri de poziția în topic
type ParserFunc func(topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error)

// streamParsers — registry intern stream → parser.
// Inițializat în init() pentru a evita ciclul: ADR-003 introduce parsers,
// iar parser-urile concrete sunt în acest package.
var streamParsers = map[string]ParserFunc{}

func init() {
	streamParsers["telemetry"] = parseHuaweiTelemetry
	streamParsers["state"] = parseTasmotaState
	streamParsers["sensor"] = parseTasmotaSensor
	streamParsers["emeter"] = parseShellyEmeter
	streamParsers["relay"] = parseShellyRelay
	streamParsers["zigbee"] = parseZigbeeJSON
}

// Parse routes la parser-ul potrivit, sau cade pe parser-ul generic dacă
// streamID nu e cunoscut.
//
// Eroarea returnată e fatală pentru mesaj (drop). Caller-ul ar trebui să logheze
// `logging.Drop("parser failed", ...)` și să nu scrie nimic în Influx.
func Parse(streamID, topic string, payload []byte, dd *registry.DeviceDefinition, extracted map[string]string) (*ParsedTelemetry, error) {
	fn, ok := streamParsers[streamID]
	if !ok {
		return parseGeneric(topic, payload, dd, extracted)
	}
	return fn(topic, payload, dd, extracted)
}

// SupportedStreams returnează lista stream-urilor cu parser dedicat.
// Util pentru health-check / introspection.
func SupportedStreams() []string {
	out := make([]string, 0, len(streamParsers))
	for k := range streamParsers {
		out = append(out, k)
	}
	return out
}

// errMalformedPayload e eroare canonică pentru payload-uri ce nu pot fi parsate.
// Caller-ul folosește `errors.Is(err, errMalformedPayload)` dacă vrea logică
// specifică (ex: nu raporta ca stat anomalie dacă e doar payload corupt).
var errMalformedPayload = fmt.Errorf("malformed payload")
