package bridge

import "testing"

func TestTranslate(t *testing.T) {
	cases := []struct {
		name      string
		topic     string
		tenantID  int64
		wantSkip  bool
		wantTopic string
	}{
		// Shelly
		{
			name:      "shelly emeter",
			topic:     "shellies/abc123/emeter/0/power",
			tenantID:  42,
			wantTopic: "tenants/42/devices/abc123/up/emeter/0/power",
		},
		{
			name:      "shelly relay",
			topic:     "shellies/dev1/relay/0",
			tenantID:  1,
			wantTopic: "tenants/1/devices/dev1/up/relay/0",
		},
		// Tasmota / NousAT
		{
			name:      "tele STATE preserved exactly (handleMessage uses HasSuffix)",
			topic:     "tele/sensor1/STATE",
			tenantID:  7,
			wantTopic: "tenants/7/devices/sensor1/up/STATE",
		},
		{
			name:      "tele SENSOR",
			topic:     "tele/sensor1/SENSOR",
			tenantID:  7,
			wantTopic: "tenants/7/devices/sensor1/up/SENSOR",
		},
		// Zigbee2MQTT
		{
			name:      "zigbee root",
			topic:     "zigbee2mqtt/abc",
			tenantID:  1,
			wantTopic: "tenants/1/devices/abc/up/zigbee",
		},
		{
			name:      "zigbee with subpath",
			topic:     "zigbee2mqtt/abc/availability",
			tenantID:  1,
			wantTopic: "tenants/1/devices/abc/up/zigbee/availability",
		},
		// Skips
		{name: "already tenant-scoped → loop avoidance", topic: "tenants/1/devices/x/up/y", tenantID: 1, wantSkip: true},
		{name: "unknown vendor", topic: "homeassistant/sensor/x/state", tenantID: 1, wantSkip: true},
		{name: "topic too short", topic: "shellies", tenantID: 1, wantSkip: true},
		{name: "empty serial", topic: "shellies//state", tenantID: 1, wantSkip: true},
		{name: "shelly missing subpath", topic: "shellies/dev1", tenantID: 1, wantSkip: true},
		{name: "tele missing suffix", topic: "tele/dev1", tenantID: 1, wantSkip: true},
		{name: "invalid tenant_id zero", topic: "shellies/x/y", tenantID: 0, wantSkip: true},
		{name: "invalid tenant_id negative", topic: "shellies/x/y", tenantID: -1, wantSkip: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Translate(tc.topic, tc.tenantID)
			if r.Skip != tc.wantSkip {
				t.Fatalf("Skip = %v, want %v (reason=%q)", r.Skip, tc.wantSkip, r.Reason)
			}
			if !tc.wantSkip && r.NewTopic != tc.wantTopic {
				t.Errorf("NewTopic = %q, want %q", r.NewTopic, tc.wantTopic)
			}
		})
	}
}

func TestLegacyPatterns(t *testing.T) {
	patterns := LegacyPatterns()
	if len(patterns) == 0 {
		t.Fatal("expected at least one legacy pattern")
	}
	expected := map[string]bool{
		"shellies/+/#":     true,
		"tele/+/#":         true,
		"zigbee2mqtt/+":    true,
		"zigbee2mqtt/+/#":  true,
	}
	for _, p := range patterns {
		if !expected[p] {
			t.Errorf("unexpected pattern: %s", p)
		}
	}
}
