// Package registry încarcă și validează Device Definitions YAML din configs/devices/.
//
// Device Definition (DD) este sursa unică de adevăr pentru:
//   - identificarea unui device pe baza topic MQTT (Faza 3 Topic Matcher)
//   - parsing-ul payload-ului (Faza 4 Parser Engine)
//   - capabilities semantice (Faza 5 Capability Engine)
//   - comenzile downlink (Faza 7 Command Engine)
//
// Toate DD-urile sunt încărcate la startup și cache-uite în memorie.
// Schema e versionată via header `schema_version`.
//
// Vezi: docs/adr/ADR-001-yaml-driven-devices.md
package registry

import "time"

// CurrentSchemaVersion e versiunea acceptată de loader; orice altă valoare e respinsă.
const CurrentSchemaVersion = "1.0"

// SupportedProtocols enumera protocoalele permise în câmpul `protocol`.
var SupportedProtocols = map[string]bool{
	"mqtt":       true,
	"modbus_tcp": false, // implementare planificată în Faza 7.5
	"http":       false, // planificat
	"coap":       false, // planificat
}

// SupportedParserTypes enumera tipurile de parser permise în câmpul `parser.type`.
// Implementarea reală a parser-elor e în Faza 4 (`internal/parsers/`).
var SupportedParserTypes = map[string]bool{
	"json":                          true,
	"json_with_measurements_array":  true, // Huawei SUN2000 style
	"raw":                           true, // payload string brut → un singur field
	"keyvalue":                      true, // ex: "k1=v1,k2=v2"
}

// DeviceDefinition reprezintă un fișier YAML din configs/devices/.
//
// Tag-urile yaml: sunt necesare pentru gopkg.in/yaml.v3 unmarshal.
// Tag-urile json: facilitează export prin API (Faza 5 expune /api/v1/registry/).
type DeviceDefinition struct {
	SchemaVersion    string                `yaml:"schema_version"     json:"schema_version"`
	ID               string                `yaml:"id"                 json:"id"`
	Name             string                `yaml:"name"               json:"name"`
	Vendor           string                `yaml:"vendor"             json:"vendor"`
	Model            string                `yaml:"model,omitempty"    json:"model,omitempty"`
	Description      string                `yaml:"description,omitempty" json:"description,omitempty"`
	Protocol         string                `yaml:"protocol"           json:"protocol"`
	Identification   IdentificationSpec    `yaml:"identification"     json:"identification"`
	Parser           ParserSpec            `yaml:"parser"             json:"parser"`
	Capabilities     []string              `yaml:"capabilities"       json:"capabilities"`
	NormalizedFields map[string]NormSpec   `yaml:"normalized_fields,omitempty" json:"normalized_fields,omitempty"`
	Commands         map[string]CommandSpec `yaml:"commands,omitempty" json:"commands,omitempty"`
	TelemetryStreams map[string]StreamSpec  `yaml:"telemetry_streams,omitempty" json:"telemetry_streams,omitempty"`

	// Internal — populat de loader la încărcare, nu prezent în YAML.
	SourcePath           string    `yaml:"-" json:"source_path,omitempty"`
	LoadedAt             time.Time `yaml:"-" json:"loaded_at,omitempty"`
	ResolvedCapabilities []string  `yaml:"-" json:"resolved_capabilities,omitempty"` // Capabilities + inherited (Faza 5)
}

// IdentificationSpec — reguli pentru a lega un mesaj MQTT primit de acest DD.
// Faza 3 Topic Matcher consumă acest block.
type IdentificationSpec struct {
	TopicMatch []TopicMatchSpec `yaml:"topic_match" json:"topic_match"`
}

// TopicMatchSpec — un pattern individual de matching topic + reguli extragere
// + identificator stream logic.
//
// Pattern syntax acceptată (interpretată în Faza 3 — internal/matcher):
//   - "+" wildcard single-level (MQTT-style): "tele/+/SENSOR"
//   - "#" wildcard multi-level (TREBUIE ultim segment): "tenants/+/devices/+/up/#"
//   - regex literal când prefix cu `~`: "~^/(?P<sn>\\d+)/.*/telemetry$"
//
// Capture groups (`+` poziții pentru MQTT, regex groups pentru ~) se extrag
// în map prin câmpul `extract` care leagă numele variabilei la indexul (`$N`)
// sau numele grupului regex.
//
// Stream e identificatorul logic al tipului de mesaj — folosit de dispatcher
// în cmd/main.go pentru a decide handler-ul (telemetry / state / sensor /
// cmd_ack / shadow / ota / emeter / relay / zigbee).
//
// Exemplu YAML:
//
//	topic_match:
//	  - pattern: "tele/+/SENSOR"
//	    stream: "sensor"
//	    extract: { device_id: "$1" }
//	  - pattern: "~^/(?P<sn>\\d+)/.*/telemetry$"
//	    stream: "telemetry"
//	    extract: { device_id: "sn" }
type TopicMatchSpec struct {
	Pattern string            `yaml:"pattern" json:"pattern"`
	Stream  string            `yaml:"stream,omitempty" json:"stream,omitempty"`
	Extract map[string]string `yaml:"extract,omitempty" json:"extract,omitempty"`
}

// ParserSpec — config pentru parser-ul de payload (Faza 4).
type ParserSpec struct {
	Type string `yaml:"type" json:"type"` // json | json_with_measurements_array | raw | keyvalue

	// PayloadPath — pentru `json_with_measurements_array`, calea spre array
	// (ex: "measurements"). Ignored pentru alte tipuri.
	PayloadPath string `yaml:"payload_path,omitempty" json:"payload_path,omitempty"`

	// MeasurementKeyField — pentru json_with_measurements_array, numele field-ului
	// din fiecare element care e cheia metric-ului (default "key").
	MeasurementKeyField string `yaml:"measurement_key_field,omitempty" json:"measurement_key_field,omitempty"`

	// MeasurementValueField — analog pentru valoare (default "value").
	MeasurementValueField string `yaml:"measurement_value_field,omitempty" json:"measurement_value_field,omitempty"`
}

// NormSpec — mapping de la field source (vendor name) la canonical name + unit.
//
// Exemplu YAML:
//
//	normalized_fields:
//	  solar_power_kw:
//	    source: "pv_input_power"
//	    unit:   "kW"
//	    multiplier: 1.0
type NormSpec struct {
	Source     string  `yaml:"source"     json:"source"`
	Unit       string  `yaml:"unit,omitempty" json:"unit,omitempty"`
	Multiplier float64 `yaml:"multiplier,omitempty" json:"multiplier,omitempty"` // default 1.0
	Decimals   *int    `yaml:"decimals,omitempty" json:"decimals,omitempty"`     // pointer ca să distingem 0 explicit de absent
}

// CommandSpec — definirea unei comenzi downlink (Faza 7).
//
// Exemplu YAML:
//
//	commands:
//	  relay_on:
//	    topic: "cmnd/{device_id}/POWER"
//	    payload: "ON"
//	    timeout_s: 5
//	    confirms_with_field: "relay_on"
//	    confirms_with_value: 1
type CommandSpec struct {
	Topic              string `yaml:"topic" json:"topic"`
	Payload            string `yaml:"payload" json:"payload"`
	TimeoutSeconds     int    `yaml:"timeout_s,omitempty" json:"timeout_s,omitempty"`
	ConfirmsWithField  string `yaml:"confirms_with_field,omitempty" json:"confirms_with_field,omitempty"`
	ConfirmsWithValue  any    `yaml:"confirms_with_value,omitempty" json:"confirms_with_value,omitempty"`
}

// StreamSpec — metadata pentru un telemetry stream (Faza 6 runtime hints).
type StreamSpec struct {
	IntervalHint string `yaml:"interval_hint,omitempty" json:"interval_hint,omitempty"` // "30s", "5m"
	OfflineAfter string `yaml:"offline_after,omitempty" json:"offline_after,omitempty"` // "2m" — runtime offline
}

// Registry — colecție in-memory de DD-uri cu lookup după ID.
// Thread-safe pentru read post-load (load se face o singură dată la startup).
type Registry struct {
	defs map[string]*DeviceDefinition
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{defs: make(map[string]*DeviceDefinition)}
}

// Get returns the device definition by ID, or nil if not found.
func (r *Registry) Get(id string) *DeviceDefinition {
	return r.defs[id]
}

// All returns all device definitions in registry, in arbitrary order.
func (r *Registry) All() []*DeviceDefinition {
	out := make([]*DeviceDefinition, 0, len(r.defs))
	for _, dd := range r.defs {
		out = append(out, dd)
	}
	return out
}

// Count returns the number of definitions loaded.
func (r *Registry) Count() int {
	return len(r.defs)
}

// ByCapability returns all definitions that declare OR inherit the given capability.
// Folosește ResolvedCapabilities populat la load (Faza 5 inheritance expansion).
//
// Exemplu: ByCapability("relay") returneaza atât DD-uri care declară "relay" direct,
// cat și cele care declară "smart_plug" (inherit relay) sau "ev_charger" etc.
func (r *Registry) ByCapability(cap string) []*DeviceDefinition {
	var out []*DeviceDefinition
	for _, dd := range r.defs {
		// Preferăm Resolved (include inheritance) dacă e populat.
		caps := dd.ResolvedCapabilities
		if len(caps) == 0 {
			caps = dd.Capabilities // fallback pentru DD-uri vechi nepopulate
		}
		for _, c := range caps {
			if c == cap {
				out = append(out, dd)
				break
			}
		}
	}
	return out
}
