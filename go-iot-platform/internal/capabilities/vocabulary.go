// Package capabilities defineste vocabularul canonical de capabilities și
// suportă inheritance ("smart_plug" inherits "relay" + "power_meter").
//
// Folosit de:
//   - registry: validare la load că DD-urile declară doar capabilities cunoscute
//   - frontend: query semantic pe capabilities (nu pe device_type)
//   - rules engine: condiții pe capabilities (Faza 8)
//
// Vezi: docs/adr/ADR-004-capability-vocabulary.md
package capabilities

// Capability — string canonical (lowercase, snake_case).
type Capability = string

// Constante pentru vocabular canonical v1.
const (
	// ── Energy ────────────────────────────────────────────────────────────
	Inverter    Capability = "inverter"
	Battery     Capability = "battery"
	SolarPV     Capability = "solar_pv"
	PowerMeter  Capability = "power_meter"
	SmartMeter  Capability = "smart_meter"

	// ── Switching ─────────────────────────────────────────────────────────
	Relay  Capability = "relay"
	Dimmer Capability = "dimmer"
	Light  Capability = "light"
	Cover  Capability = "cover"
	Valve  Capability = "valve"

	// ── Sensing ───────────────────────────────────────────────────────────
	TemperatureSensor Capability = "temperature_sensor"
	HumiditySensor    Capability = "humidity_sensor"
	MotionSensor      Capability = "motion_sensor"
	DoorSensor        Capability = "door_sensor"
	PressureSensor    Capability = "pressure_sensor"
	CO2Sensor         Capability = "co2_sensor"
	LuxSensor         Capability = "lux_sensor"

	// ── Composite (cu inheritance) ────────────────────────────────────────
	SmartPlug       Capability = "smart_plug"        // = relay + power_meter
	HybridInverter  Capability = "hybrid_inverter"   // = inverter + battery
	ClimateSensor   Capability = "climate_sensor"    // = temperature_sensor + humidity_sensor
	EVCharger       Capability = "ev_charger"        // = relay + power_meter

	// ── Connectivity (meta) ───────────────────────────────────────────────
	BatteryPowered Capability = "battery_powered"
	MainsPowered   Capability = "mains_powered"
	WiFi           Capability = "wifi"
	Zigbee         Capability = "zigbee"
	LoRa           Capability = "lora"
	ModbusTCP      Capability = "modbus_tcp"
)

// Vocabulary — toate capability-urile cunoscute, cu descriere short.
// Modificare = ADR nou + review code.
var Vocabulary = map[Capability]string{
	// Energy
	Inverter:   "DC↔AC converter (PV, hybrid, off-grid)",
	Battery:    "Energy storage with SOC, temp, charge/discharge",
	SolarPV:    "Photovoltaic string (PV voltage, current)",
	PowerMeter: "Active power measurement (W, kWh)",
	SmartMeter: "Power meter with bidirectional energy (imported/exported)",

	// Switching
	Relay:  "Binary ON/OFF switch",
	Dimmer: "Variable 0-100% control",
	Light:  "Light source (dimmer + color management)",
	Cover:  "Blinds/shutters with position 0-100%",
	Valve:  "Water/gas valve ON/OFF with fail-safe",

	// Sensing
	TemperatureSensor: "Temperature in °C",
	HumiditySensor:    "Relative humidity in %",
	MotionSensor:      "PIR motion detection (boolean)",
	DoorSensor:        "Open/closed contact sensor",
	PressureSensor:    "Atmospheric or fluid pressure (hPa, bar)",
	CO2Sensor:         "Carbon dioxide concentration (ppm)",
	LuxSensor:         "Illumination (lux/lumens)",

	// Composite
	SmartPlug:      "WiFi smart plug = relay + power_meter",
	HybridInverter: "Inverter + battery = inverter + battery",
	ClimateSensor:  "Temp + humidity = temperature_sensor + humidity_sensor",
	EVCharger:      "EV charge station = relay + power_meter",

	// Connectivity
	BatteryPowered: "Internal battery powered (low duty cycle)",
	MainsPowered:   "Mains powered (always-on)",
	WiFi:           "WiFi 2.4/5 GHz transport",
	Zigbee:         "Zigbee 3.0 transport",
	LoRa:           "LoRaWAN transport",
	ModbusTCP:      "Modbus TCP/IP transport",
}

// IsKnown returneaza true daca cap e in vocabular.
func IsKnown(cap Capability) bool {
	_, ok := Vocabulary[cap]
	return ok
}

// AllKnown returneaza lista capabilities cunoscute (ordine arbitrara).
// Util pentru introspection / API health check.
func AllKnown() []Capability {
	out := make([]Capability, 0, len(Vocabulary))
	for c := range Vocabulary {
		out = append(out, c)
	}
	return out
}
