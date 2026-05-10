package matcher

import (
	"path/filepath"
	"testing"

	"go-iot-platform/internal/registry"
)

// helpers — registry minimal cu un singur DD pt teste izolate.

func ddWith(id string, patterns ...registry.TopicMatchSpec) *registry.DeviceDefinition {
	return &registry.DeviceDefinition{
		SchemaVersion: registry.CurrentSchemaVersion,
		ID:            id,
		Name:          "Test " + id,
		Protocol:      "mqtt",
		Identification: registry.IdentificationSpec{
			TopicMatch: patterns,
		},
		Parser:       registry.ParserSpec{Type: "json"},
		Capabilities: []string{"relay"}, // canonical capability (vocab gating Faza 5)
	}
}

// (registry doesn't expose direct insertion to preserve "only validated DDs"
// invariant; tests use newRegistryViaYAML to serialize → load → validate full path)

// ────────────────────────────────────────────────────────────────────────────
// MQTT wildcard tests
// ────────────────────────────────────────────────────────────────────────────

func TestMQTTExactMatch(t *testing.T) {
	dd := ddWith("d1", registry.TopicMatchSpec{
		Pattern: "tele/abc/STATE", Stream: "state",
	})
	m := mustMatcher(t, dd)
	if got := m.Match("tele/abc/STATE"); got == nil || got.Definition.ID != "d1" {
		t.Errorf("expected match; got %+v", got)
	}
	if got := m.Match("tele/abc/SENSOR"); got != nil {
		t.Errorf("should not match different topic; got %+v", got)
	}
}

func TestMQTTSinglePlus(t *testing.T) {
	dd := ddWith("d1", registry.TopicMatchSpec{
		Pattern: "tele/+/STATE", Stream: "state",
		Extract: map[string]string{"device_id": "$1"},
	})
	m := mustMatcher(t, dd)

	cases := map[string]string{
		"tele/abc/STATE":         "abc",
		"tele/sensor_42/STATE":   "sensor_42",
		"tele/foo-bar.baz/STATE": "foo-bar.baz",
	}
	for topic, expectedID := range cases {
		got := m.Match(topic)
		if got == nil {
			t.Errorf("topic %q: no match", topic)
			continue
		}
		if got.Extracted["device_id"] != expectedID {
			t.Errorf("topic %q: device_id=%q, want %q",
				topic, got.Extracted["device_id"], expectedID)
		}
	}
}

func TestMQTTMultiPlus(t *testing.T) {
	dd := ddWith("d1", registry.TopicMatchSpec{
		Pattern: "tenants/+/devices/+/up/state", Stream: "state",
		Extract: map[string]string{"tenant_id": "$1", "device_id": "$2"},
	})
	m := mustMatcher(t, dd)

	got := m.Match("tenants/2/devices/abc/up/state")
	if got == nil {
		t.Fatal("expected match")
	}
	if got.Extracted["tenant_id"] != "2" {
		t.Errorf("tenant_id=%q want 2", got.Extracted["tenant_id"])
	}
	if got.Extracted["device_id"] != "abc" {
		t.Errorf("device_id=%q want abc", got.Extracted["device_id"])
	}
}

func TestMQTTHashWildcard(t *testing.T) {
	dd := ddWith("d1", registry.TopicMatchSpec{
		Pattern: "tenants/+/devices/+/up/#", Stream: "any",
		Extract: map[string]string{"tenant_id": "$1", "device_id": "$2", "rest": "$3"},
	})
	m := mustMatcher(t, dd)

	cases := map[string]string{
		"tenants/2/devices/abc/up/state":          "state",
		"tenants/2/devices/abc/up/sensor":         "sensor",
		"tenants/2/devices/abc/up/cmd_ack":        "cmd_ack",
		"tenants/2/devices/abc/up/foo/bar/baz":    "foo/bar/baz",
	}
	for topic, expectedRest := range cases {
		got := m.Match(topic)
		if got == nil {
			t.Errorf("topic %q: no match", topic)
			continue
		}
		if got.Extracted["rest"] != expectedRest {
			t.Errorf("topic %q: rest=%q want %q",
				topic, got.Extracted["rest"], expectedRest)
		}
	}
}

func TestMQTTNoMatch(t *testing.T) {
	dd := ddWith("d1", registry.TopicMatchSpec{
		Pattern: "tele/+/STATE", Stream: "state",
	})
	m := mustMatcher(t, dd)

	noMatches := []string{
		"tele/abc",                  // missing /STATE
		"tele/abc/STATE/extra",      // extra segment
		"shellies/abc/relay/0",      // different prefix
		"",                          // empty
		"tele//STATE",               // empty + segment
	}
	for _, topic := range noMatches {
		if got := m.Match(topic); got != nil {
			t.Errorf("topic %q: should NOT match; got %+v", topic, got)
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Regex (~ prefix) tests
// ────────────────────────────────────────────────────────────────────────────

func TestRegexNamedGroup(t *testing.T) {
	dd := ddWith("huawei", registry.TopicMatchSpec{
		Pattern: `~^/(?P<sn>\d+)/[^/]+/[^/]+/telemetry$`,
		Stream:  "telemetry",
		Extract: map[string]string{"device_id": "sn"},
	})
	m := mustMatcher(t, dd)

	got := m.Match("/39371381/dtsalunis3/1oktkm1/telemetry")
	if got == nil {
		t.Fatal("expected match")
	}
	if got.Extracted["device_id"] != "39371381" {
		t.Errorf("device_id=%q want 39371381", got.Extracted["device_id"])
	}
}

func TestRegexPositionalGroup(t *testing.T) {
	dd := ddWith("d1", registry.TopicMatchSpec{
		Pattern: `~^prefix/(\w+)/(\d+)$`,
		Stream:  "x",
		Extract: map[string]string{"name": "$1", "num": "$2"},
	})
	m := mustMatcher(t, dd)

	got := m.Match("prefix/foo/42")
	if got == nil {
		t.Fatal("expected match")
	}
	if got.Extracted["name"] != "foo" || got.Extracted["num"] != "42" {
		t.Errorf("extracted=%+v", got.Extracted)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Multi-pattern, multi-DD priority
// ────────────────────────────────────────────────────────────────────────────

func TestFirstMatchWins(t *testing.T) {
	// Two DDs with overlapping patterns; alphabetical order means a_first wins.
	d1 := ddWith("a_first", registry.TopicMatchSpec{
		Pattern: "tenants/+/devices/+/up/state", Stream: "state",
	})
	d2 := ddWith("b_second", registry.TopicMatchSpec{
		Pattern: "tenants/+/devices/+/up/state", Stream: "state-alt",
	})
	m := mustMatcher(t, d1, d2)

	got := m.Match("tenants/2/devices/abc/up/state")
	if got == nil || got.Definition.ID != "a_first" {
		t.Errorf("expected a_first to win; got %+v", got)
	}
}

func TestMultiPatternPerDD(t *testing.T) {
	dd := ddWith("nous",
		registry.TopicMatchSpec{Pattern: "tele/+/STATE", Stream: "state"},
		registry.TopicMatchSpec{Pattern: "tele/+/SENSOR", Stream: "sensor"},
		registry.TopicMatchSpec{Pattern: "tenants/+/devices/+/up/state", Stream: "state"},
	)
	m := mustMatcher(t, dd)

	tests := map[string]string{
		"tele/abc/STATE":                       "state",
		"tele/abc/SENSOR":                      "sensor",
		"tenants/1/devices/abc/up/state":       "state",
	}
	for topic, expectedStream := range tests {
		got := m.Match(topic)
		if got == nil {
			t.Errorf("topic %q: no match", topic)
			continue
		}
		if got.Stream != expectedStream {
			t.Errorf("topic %q: stream=%q want %q", topic, got.Stream, expectedStream)
		}
	}
}

// (Compile error tests sunt în registry_test.go: TestRejectInvalidRegexInTopicMatch
//  și TestRejectInvalidWildcardPlacement — registry validator catches them upfront,
//  matcher nu le vede niciodată; aceasta e defense-in-depth corectă.)

// ────────────────────────────────────────────────────────────────────────────
// Edge cases
// ────────────────────────────────────────────────────────────────────────────

func TestEmptyRegistry(t *testing.T) {
	m, errs := New(registry.NewRegistry())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if m.Count() != 0 {
		t.Errorf("expected 0 patterns, got %d", m.Count())
	}
	if got := m.Match("anything"); got != nil {
		t.Errorf("empty matcher should match nothing; got %+v", got)
	}
}

func TestNilRegistry(t *testing.T) {
	m, errs := New(nil)
	if len(errs) != 0 || m.Count() != 0 {
		t.Errorf("nil registry should give empty matcher")
	}
	if got := m.Match("anything"); got != nil {
		t.Errorf("expected nil match")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Production smoke test — încarcă configs/devices/ real și matchează topice
// ────────────────────────────────────────────────────────────────────────────

func TestProductionMatching(t *testing.T) {
	prodDir := filepath.Join("..", "..", "..", "configs", "devices")
	reg, errs, err := registry.LoadDir(prodDir)
	if err != nil {
		t.Skipf("configs/devices/ not available: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("registry errors: %v", errs)
	}

	m, mErrs := New(reg)
	if len(mErrs) > 0 {
		t.Fatalf("matcher compile errors: %v", mErrs)
	}

	cases := []struct {
		topic       string
		wantDDID    string
		wantStream  string
	}{
		{"tele/vd2ap14_boiler/SENSOR", "nous_a1t", "sensor"},
		{"tele/vd2ap14_boiler/STATE", "nous_a1t", "state"},
		{"tenants/2/devices/vd2ap14_boiler/up/sensor", "nous_a1t", "sensor"},
		{"tenants/2/devices/vd2ap14_boiler/up/state", "nous_a1t", "state"},
		{"/39371381/dtsalunis3/1oktkm1/telemetry", "huawei_sun2000_3phase", "telemetry"},
		{"tenants/2/devices/39371381/up/telemetry", "huawei_sun2000_3phase", "telemetry"},
		{"tenants/2/devices/39371381/up/shadow", "huawei_sun2000_3phase", "shadow"},
		{"shellies/shellyem-ABC/emeter/0/power", "shelly_em", "emeter"},
		{"shellies/shellyem-ABC/relay/0", "shelly_em", "relay"},
		{"zigbee2mqtt/livingroom_temp", "zigbee_temperature", "zigbee"},
	}

	for _, tc := range cases {
		got := m.Match(tc.topic)
		if got == nil {
			t.Errorf("topic %q: no match", tc.topic)
			continue
		}
		if got.Definition.ID != tc.wantDDID {
			t.Errorf("topic %q: dd=%q want %q", tc.topic, got.Definition.ID, tc.wantDDID)
		}
		if got.Stream != tc.wantStream {
			t.Errorf("topic %q: stream=%q want %q", tc.topic, got.Stream, tc.wantStream)
		}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Benchmark — ținta < 50µs/op pentru ~10 patterns
// ────────────────────────────────────────────────────────────────────────────

func BenchmarkMatcher(b *testing.B) {
	prodDir := filepath.Join("..", "..", "..", "configs", "devices")
	reg, _, err := registry.LoadDir(prodDir)
	if err != nil {
		b.Skipf("configs/devices/ not available: %v", err)
	}
	m, _ := New(reg)

	topics := []string{
		"tele/vd2ap14_boiler/SENSOR",
		"tenants/2/devices/39371381/up/telemetry",
		"shellies/shellyem-ABC/emeter/0/power",
		"zigbee2mqtt/livingroom_temp",
		"unknown/topic/that/wont/match", // worst case: all patterns scanned
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, t := range topics {
			_ = m.Match(t)
		}
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// mustMatcher creates a matcher from inline DD definitions. Uses YAML round-trip
// because Registry doesn't expose direct insertion (preserves invariant: only
// validated DD-uri intră în registry).
func mustMatcher(t *testing.T, defs ...*registry.DeviceDefinition) *Matcher {
	t.Helper()
	reg := newRegistryViaYAML(t, defs...)
	m, errs := New(reg)
	if len(errs) > 0 {
		t.Fatalf("matcher errors: %v", errs)
	}
	return m
}

// newRegistryViaYAML serializează DD-urile la YAML pe disk, apoi le încarcă
// înapoi prin LoadDir. Asta menține invariantul: doar YAML validat intră în reg.
func newRegistryViaYAML(t *testing.T, defs ...*registry.DeviceDefinition) *registry.Registry {
	t.Helper()
	dir := t.TempDir()
	for i, dd := range defs {
		yamlPath := filepath.Join(dir, dd.ID+".yaml")
		writeYAML(t, yamlPath, dd, i)
	}
	reg, errs, err := registry.LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("LoadDir non-fatal errs: %v", errs)
	}
	return reg
}
