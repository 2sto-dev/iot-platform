package capabilities

import (
	"reflect"
	"sort"
	"testing"
)

func TestVocabularyComplete(t *testing.T) {
	// Toate constants definite trebuie să fie în Vocabulary.
	required := []Capability{
		Inverter, Battery, SolarPV, PowerMeter, SmartMeter,
		Relay, Dimmer, Light, Cover, Valve,
		TemperatureSensor, HumiditySensor, MotionSensor, DoorSensor,
		PressureSensor, CO2Sensor, LuxSensor,
		SmartPlug, HybridInverter, ClimateSensor, EVCharger,
		BatteryPowered, MainsPowered, WiFi, Zigbee, LoRa, ModbusTCP,
	}
	for _, c := range required {
		if !IsKnown(c) {
			t.Errorf("required capability %q not in Vocabulary", c)
		}
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown(Relay) {
		t.Error("relay should be known")
	}
	if IsKnown("nonexistent_cap") {
		t.Error("unknown cap should not be known")
	}
}

func TestResolveSimple(t *testing.T) {
	// No inheritance — output = input
	got := Resolve([]Capability{Relay, WiFi})
	want := []Capability{Relay, WiFi}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestResolveSmartPlug(t *testing.T) {
	got := Resolve([]Capability{SmartPlug})
	// smart_plug → relay + power_meter
	wantSet := map[Capability]bool{
		SmartPlug:  true,
		Relay:      true,
		PowerMeter: true,
	}
	if len(got) != 3 {
		t.Errorf("expected 3 capabilities, got %d: %v", len(got), got)
	}
	for _, c := range got {
		if !wantSet[c] {
			t.Errorf("unexpected capability %q in resolve output", c)
		}
	}
}

func TestResolveHybridInverter(t *testing.T) {
	got := Resolve([]Capability{HybridInverter})
	// hybrid_inverter → inverter + battery
	wantSet := map[Capability]bool{HybridInverter: true, Inverter: true, Battery: true}
	for _, c := range got {
		if !wantSet[c] {
			t.Errorf("unexpected: %s", c)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d, want 3", len(got))
	}
}

func TestResolveClimateSensor(t *testing.T) {
	got := Resolve([]Capability{ClimateSensor})
	expected := map[Capability]bool{
		ClimateSensor:     true,
		TemperatureSensor: true,
		HumiditySensor:    true,
	}
	if len(got) != 3 {
		t.Errorf("got %d capabilities, want 3", len(got))
	}
	for _, c := range got {
		if !expected[c] {
			t.Errorf("unexpected: %s", c)
		}
	}
}

func TestResolveSmartMeter(t *testing.T) {
	// smart_meter → power_meter (single parent)
	got := Resolve([]Capability{SmartMeter})
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}

func TestResolveDeduplicate(t *testing.T) {
	// Declared deja conține parintele — nu ar trebui duplicate.
	got := Resolve([]Capability{SmartPlug, Relay, PowerMeter})
	if len(got) != 3 {
		t.Errorf("duplicate not removed: got %v (expected 3 unique)", got)
	}
}

func TestResolveMultipleComposite(t *testing.T) {
	got := Resolve([]Capability{SmartPlug, ClimateSensor})
	// smart_plug → relay + power_meter
	// climate_sensor → temperature_sensor + humidity_sensor
	expected := []Capability{
		SmartPlug, ClimateSensor,
		Relay, PowerMeter,
		TemperatureSensor, HumiditySensor,
	}
	if len(got) != len(expected) {
		t.Errorf("got %d, want %d", len(got), len(expected))
	}
	gotSet := map[Capability]bool{}
	for _, c := range got {
		gotSet[c] = true
	}
	for _, e := range expected {
		if !gotSet[e] {
			t.Errorf("missing %s", e)
		}
	}
}

func TestResolveLight(t *testing.T) {
	// Multi-level: light → dimmer (single parent)
	got := Resolve([]Capability{Light})
	if len(got) != 2 {
		t.Errorf("light should resolve to 2 (light + dimmer), got %d: %v", len(got), got)
	}
}

func TestResolveStrictUnknown(t *testing.T) {
	_, err := ResolveStrict([]Capability{"unknown_cap_xyz"})
	if err == nil {
		t.Error("expected error for unknown capability")
	}
}

func TestResolveStrictKnown(t *testing.T) {
	out, err := ResolveStrict([]Capability{SmartPlug})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("unexpected resolved count: %d", len(out))
	}
}

func TestInherits(t *testing.T) {
	cases := []struct {
		child, parent Capability
		want          bool
	}{
		{SmartPlug, Relay, true},
		{SmartPlug, PowerMeter, true},
		{SmartPlug, Battery, false},
		{HybridInverter, Inverter, true},
		{HybridInverter, Battery, true},
		{HybridInverter, Relay, false},
		{ClimateSensor, TemperatureSensor, true},
		{ClimateSensor, HumiditySensor, true},
		{Relay, SmartPlug, false}, // Direction matters: relay nu inherit smart_plug
		{Inverter, Inverter, true}, // self-inherit (degenerate but consistent)
	}
	for _, tc := range cases {
		got := Inherits(tc.child, tc.parent)
		if got != tc.want {
			t.Errorf("Inherits(%s, %s) = %v, want %v", tc.child, tc.parent, got, tc.want)
		}
	}
}

func TestAllKnownNonEmpty(t *testing.T) {
	all := AllKnown()
	if len(all) < 20 {
		t.Errorf("AllKnown() should return ≥20, got %d", len(all))
	}
	// Verify deterministic Sort doesn't break (sanity)
	sort.Strings(all)
	if len(all) != len(AllKnown()) {
		t.Error("AllKnown() returns inconsistent count")
	}
}

func TestResolveEmpty(t *testing.T) {
	got := Resolve([]Capability{})
	if len(got) != 0 {
		t.Errorf("empty input should give empty output; got %v", got)
	}
	got = Resolve(nil)
	if len(got) != 0 {
		t.Errorf("nil input should give empty output; got %v", got)
	}
}
