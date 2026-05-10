package parsers

import (
	"testing"
	"time"
)

// ============================================================================
// Tasmota STATE
// ============================================================================

func TestTasmotaState_Basic(t *testing.T) {
	cases := []struct {
		name      string
		payload   string
		wantOn    int
		wantState string
	}{
		{"on uppercase", `{"POWER":"ON","RSSI":42}`, 1, "ON"},
		{"off uppercase", `{"POWER":"OFF","RSSI":42}`, 0, "OFF"},
		{"on lowercase", `{"POWER":"on","RSSI":42}`, 1, "on"},
		{"off lowercase", `{"POWER":"off","RSSI":42}`, 0, "off"},
		{"missing rssi", `{"POWER":"ON"}`, 1, "ON"},
		{"with extras", `{"POWER":"ON","RSSI":42,"Heap":24,"Wifi":{"SSId":"X"}}`, 1, "ON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pt, err := parseTasmotaState("tele/x/STATE", []byte(tc.payload), nil, nil)
			if err != nil {
				t.Fatalf("parse err: %v", err)
			}
			if pt.Source != "nousat" || pt.Type != "state" {
				t.Errorf("source/type = %s/%s, want nousat/state", pt.Source, pt.Type)
			}
			if pt.Fields["relay_on"] != tc.wantOn {
				t.Errorf("relay_on = %v, want %d", pt.Fields["relay_on"], tc.wantOn)
			}
			if pt.Fields["relay_state"] != tc.wantState {
				t.Errorf("relay_state = %v, want %q", pt.Fields["relay_state"], tc.wantState)
			}
		})
	}
}

func TestTasmotaState_Malformed(t *testing.T) {
	bad := [][]byte{
		[]byte(`not json`),
		[]byte(`{"POWER":}`), // truncated
		[]byte(``),           // empty
		nil,
	}
	for _, b := range bad {
		if _, err := parseTasmotaState("tele/x/STATE", b, nil, nil); err == nil {
			t.Errorf("expected error for payload %q", b)
		}
	}
}

// ============================================================================
// Tasmota SENSOR
// ============================================================================

func TestTasmotaSensor_FullPayload(t *testing.T) {
	payload := `{
		"Time":"2026-05-10T10:30:37",
		"ENERGY":{
			"Total":1822.913,
			"Today":0.875,
			"Yesterday":3.128,
			"Power":1500,
			"ApparentPower":1580,
			"ReactivePower":250,
			"Factor":0.95,
			"Voltage":230,
			"Current":6.5
		}
	}`
	pt, err := parseTasmotaSensor("tele/x/SENSOR", []byte(payload), nil, nil)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if pt.Source != "nousat" || pt.Type != "energy" {
		t.Errorf("source/type")
	}
	checks := map[string]float64{
		"nousat_power":          1500,
		"nousat_apparent_power": 1580,
		"nousat_reactive_power": 250,
		"nousat_power_factor":   0.95,
		"nousat_voltage":        230,
		"nousat_current":        6.5,
		"nousat_total":          1822.913,
		"nousat_today":          0.875,
		"nousat_yesterday":      3.128,
	}
	for k, want := range checks {
		got, ok := pt.Fields[k].(float64)
		if !ok {
			t.Errorf("%s missing or wrong type: %v", k, pt.Fields[k])
			continue
		}
		if got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
}

func TestTasmotaSensor_StandbyValues(t *testing.T) {
	payload := `{"Time":"2026-05-10T10:30:37","ENERGY":{"Total":1822.913,"Today":0.875,"Power":2,"Voltage":249,"Current":0.013,"Factor":0.46}}`
	pt, err := parseTasmotaSensor("tele/x/SENSOR", []byte(payload), nil, nil)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if pt.Fields["nousat_power"].(float64) != 2 {
		t.Errorf("standby power")
	}
}

func TestTasmotaSensor_TimestampParsed(t *testing.T) {
	// Tasmota timestamp format e ISO 8601 fără timezone — RFC3339 parse fails,
	// fallback la time.Now() (validăm că nu e zero time).
	payload := `{"Time":"2026-05-10T10:30:37","ENERGY":{"Power":100}}`
	pt, _ := parseTasmotaSensor("tele/x/SENSOR", []byte(payload), nil, nil)
	if pt.Timestamp.IsZero() {
		t.Error("timestamp should never be zero (fallback time.Now())")
	}
	// Tasmota Time NU e RFC3339 (no Z, no offset), deci parser cade pe time.Now() — așteptat.
	if time.Since(pt.Timestamp) > time.Minute {
		t.Errorf("expected recent fallback time, got %v", pt.Timestamp)
	}
}

// ============================================================================
// Huawei SUN2000 telemetry
// ============================================================================

func TestHuaweiTelemetry_FullPayload(t *testing.T) {
	payload := `{
		"ts":"2026-05-10T10:30:37Z",
		"measurements":[
			{"key":"pv_input_power","value":8.66},
			{"key":"battery_soc","value":99},
			{"key":"battery_temp","value":55.6},
			{"key":"grid_power","value":6848},
			{"key":"daily_energy_yield","value":9.9}
		],
		"house_load_kw_est":0.51
	}`
	pt, err := parseHuaweiTelemetry("/sn/coll/dsn/telemetry", []byte(payload), nil, nil)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if pt.Source != "sun2000" || pt.Type != "solar_inverter" {
		t.Errorf("source/type = %s/%s", pt.Source, pt.Type)
	}
	if pt.Fields["pv_input_power"].(float64) != 8.66 {
		t.Errorf("pv_input_power")
	}
	if pt.Fields["battery_soc"].(float64) != 99 {
		t.Errorf("battery_soc")
	}
	if pt.Fields["house_load_kw_est"].(float64) != 0.51 {
		t.Errorf("house_load")
	}
	// Verifică că timestamp-ul e parsat din ts (nu fallback)
	expectedT, _ := time.Parse(time.RFC3339, "2026-05-10T10:30:37Z")
	if !pt.Timestamp.Equal(expectedT) {
		t.Errorf("timestamp = %v, want %v", pt.Timestamp, expectedT)
	}
}

func TestHuaweiTelemetry_NoTimestamp(t *testing.T) {
	payload := `{"measurements":[{"key":"pv_input_power","value":5}]}`
	pt, _ := parseHuaweiTelemetry("/sn/c/d/telemetry", []byte(payload), nil, nil)
	if pt.Timestamp.IsZero() {
		t.Error("expected fallback time.Now()")
	}
}

func TestHuaweiTelemetry_EmptyMeasurements(t *testing.T) {
	payload := `{"measurements":[]}`
	if _, err := parseHuaweiTelemetry("/sn/c/d/telemetry", []byte(payload), nil, nil); err == nil {
		t.Error("expected error for empty measurements + zero house_load")
	}
}

func TestHuaweiTelemetry_HouseLoadOnly(t *testing.T) {
	// Edge case: doar house_load, fără măsurători → nu fail (avem date utile)
	// Dar implementarea curentă fail pentru că `len(measurements)==0 && house_load==0`...
	// Aici house_load=0.5 != 0, deci OK
	payload := `{"measurements":[],"house_load_kw_est":0.5}`
	pt, err := parseHuaweiTelemetry("/sn/c/d/telemetry", []byte(payload), nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pt.Fields["house_load_kw_est"].(float64) != 0.5 {
		t.Errorf("house_load")
	}
}

func TestHuaweiTelemetry_SkipMalformedMeasurement(t *testing.T) {
	payload := `{"measurements":[
		{"key":"good","value":1},
		{"key":"","value":2},
		{"value":3},
		{"key":"another","value":4}
	]}`
	pt, _ := parseHuaweiTelemetry("/x/y/z/telemetry", []byte(payload), nil, nil)
	if _, ok := pt.Fields["good"]; !ok {
		t.Error("`good` should be present")
	}
	if _, ok := pt.Fields["another"]; !ok {
		t.Error("`another` should be present")
	}
	if len(pt.Fields) != 2 {
		t.Errorf("expected 2 valid fields, got %d", len(pt.Fields))
	}
}

// ============================================================================
// Shelly emeter
// ============================================================================

func TestShellyEmeter_Power(t *testing.T) {
	pt, err := parseShellyEmeter("shellies/abc/emeter/0/power", []byte("1234.56"), nil, nil)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if pt.Source != "shelly" || pt.Type != "power_meter" {
		t.Errorf("source/type")
	}
	// Field name is title-cased ("Power")
	if pt.Fields["Power"].(float64) != 1234.56 {
		t.Errorf("Power = %v", pt.Fields["Power"])
	}
}

func TestShellyEmeter_AllFields(t *testing.T) {
	cases := map[string]string{
		"power":          "Power",
		"voltage":        "Voltage",
		"current":        "Current",
		"total":          "Total",
		"total_returned": "Total_returned",
	}
	for topicField, wantKey := range cases {
		topic := "shellies/abc/emeter/0/" + topicField
		pt, err := parseShellyEmeter(topic, []byte("42.0"), nil, nil)
		if err != nil {
			t.Errorf("%s: %v", topicField, err)
			continue
		}
		if _, ok := pt.Fields[wantKey]; !ok {
			t.Errorf("%s: missing field %q (got fields %v)", topicField, wantKey, pt.Fields)
		}
	}
}

func TestShellyEmeter_InvalidFloat(t *testing.T) {
	if _, err := parseShellyEmeter("shellies/x/emeter/0/power", []byte("not-a-number"), nil, nil); err == nil {
		t.Error("expected float parse error")
	}
}

// ============================================================================
// Shelly relay
// ============================================================================

func TestShellyRelay(t *testing.T) {
	cases := map[string]int{
		"on":  1,
		"ON":  1,
		"On":  1,
		"off": 0,
		"OFF": 0,
		"":    0, // not "on"
	}
	for payload, want := range cases {
		pt, err := parseShellyRelay("shellies/abc/relay/0", []byte(payload), nil, nil)
		if err != nil {
			t.Errorf("payload %q: %v", payload, err)
			continue
		}
		if pt.Fields["state"] != want {
			t.Errorf("payload %q: state = %v, want %d", payload, pt.Fields["state"], want)
		}
	}
}

// ============================================================================
// Zigbee2MQTT JSON
// ============================================================================

func TestZigbeeJSON(t *testing.T) {
	payload := `{"temperature":21.5,"humidity":56,"battery":85,"linkquality":42,"voltage":3000}`
	pt, err := parseZigbeeJSON("zigbee2mqtt/livingroom_temp", []byte(payload), nil, nil)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if pt.Source != "zigbee2mqtt" || pt.Type != "sensor" {
		t.Errorf("source/type")
	}
	if pt.Fields["temperature"].(float64) != 21.5 {
		t.Errorf("temperature")
	}
	if pt.Fields["humidity"].(float64) != 56 {
		t.Errorf("humidity")
	}
}

func TestZigbeeJSON_EmptyPayload(t *testing.T) {
	if _, err := parseZigbeeJSON("zigbee2mqtt/x", []byte(`{}`), nil, nil); err == nil {
		t.Error("expected error for empty zigbee payload")
	}
}

func TestZigbeeJSON_Malformed(t *testing.T) {
	if _, err := parseZigbeeJSON("zigbee2mqtt/x", []byte(`not-json`), nil, nil); err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// ============================================================================
// Generic parser (fallback)
// ============================================================================

func TestGeneric_JSON(t *testing.T) {
	pt, err := parseGeneric("foo/bar", []byte(`{"key1":1,"key2":"x"}`), nil, nil)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if pt.Source != "generic" || pt.Type != "auto_detected" {
		t.Errorf("source/type")
	}
	if pt.Fields["key1"].(float64) != 1 {
		t.Errorf("key1")
	}
}

func TestGeneric_PlainNumber(t *testing.T) {
	pt, err := parseGeneric("foo", []byte(`42.5`), nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pt.Fields["value"].(float64) != 42.5 {
		t.Errorf("value")
	}
}

func TestGeneric_PlainString(t *testing.T) {
	pt, err := parseGeneric("foo", []byte(`hello`), nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pt.Fields["value"].(string) != "hello" {
		t.Errorf("value")
	}
}

func TestGeneric_EmptyPayload(t *testing.T) {
	if _, err := parseGeneric("foo", []byte(``), nil, nil); err == nil {
		t.Error("expected error for empty payload")
	}
}

// ============================================================================
// Dispatcher
// ============================================================================

func TestParseDispatcher(t *testing.T) {
	cases := []struct {
		stream  string
		payload string
		topic   string
	}{
		{"state", `{"POWER":"ON","RSSI":42}`, "tele/x/STATE"},
		{"sensor", `{"Time":"2026-05-10T10:30:37","ENERGY":{"Power":100,"Voltage":230}}`, "tele/x/SENSOR"},
		{"telemetry", `{"measurements":[{"key":"pv_input_power","value":5}]}`, "/sn/c/d/telemetry"},
		{"emeter", `42.0`, "shellies/abc/emeter/0/power"},
		{"relay", `on`, "shellies/abc/relay/0"},
		{"zigbee", `{"temperature":21.5}`, "zigbee2mqtt/x"},
	}
	for _, tc := range cases {
		t.Run(tc.stream, func(t *testing.T) {
			pt, err := Parse(tc.stream, tc.topic, []byte(tc.payload), nil, nil)
			if err != nil {
				t.Errorf("Parse %s: %v", tc.stream, err)
				return
			}
			if pt == nil {
				t.Error("nil ParsedTelemetry")
			}
		})
	}
}

func TestParseUnknownStream_FallsToGeneric(t *testing.T) {
	pt, err := Parse("unknown_stream", "foo/bar", []byte(`{"k":"v"}`), nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if pt.Source != "generic" {
		t.Errorf("expected generic fallback")
	}
}

func TestSupportedStreams(t *testing.T) {
	streams := SupportedStreams()
	if len(streams) < 5 {
		t.Errorf("expected ≥5 supported streams, got %d", len(streams))
	}
}

// ============================================================================
// Benchmark
// ============================================================================

func BenchmarkParseTasmotaSensor(b *testing.B) {
	payload := []byte(`{"Time":"2026-05-10T10:30:37","ENERGY":{"Total":1822.913,"Today":0.875,"Power":1500,"Voltage":230,"Current":6.5,"Factor":0.95}}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = parseTasmotaSensor("tele/x/SENSOR", payload, nil, nil)
	}
}

func BenchmarkParseHuaweiTelemetry(b *testing.B) {
	payload := []byte(`{"ts":"2026-05-10T10:30:37Z","measurements":[{"key":"pv_input_power","value":8.66},{"key":"battery_soc","value":99},{"key":"battery_temp","value":55.6}]}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = parseHuaweiTelemetry("/sn/c/d/telemetry", payload, nil, nil)
	}
}

func BenchmarkParseDispatcher(b *testing.B) {
	payload := []byte(`{"POWER":"ON","RSSI":42}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Parse("state", "tele/x/STATE", payload, nil, nil)
	}
}
