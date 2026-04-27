package topics

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in        string
		wantErr   bool
		legacy    bool
		tenantID  int64
		deviceID  string
		direction string
		stream    string
	}{
		{in: "tenants/1/devices/dev-001/up/telemetry", tenantID: 1, deviceID: "dev-001", direction: "up", stream: "telemetry"},
		{in: "tenants/42/devices/abc/down/cmd", tenantID: 42, deviceID: "abc", direction: "down", stream: "cmd"},
		{in: "tenants/7/devices/x/up/state/extra/path", tenantID: 7, deviceID: "x", direction: "up", stream: "state"},
		// Legacy
		{in: "shellies/1234/emeter/0/power", legacy: true},
		{in: "tele/abc/STATE", legacy: true},
		{in: "zigbee2mqtt/sensor1", legacy: true},
		// Errors
		{in: "tenants/1/devices/dev/up", wantErr: true},                   // too few segments
		{in: "tenants//devices/dev/up/telemetry", wantErr: true},          // empty tenant_id
		{in: "tenants/1/devices//up/telemetry", wantErr: true},            // empty device_id
		{in: "tenants/abc/devices/dev/up/telemetry", wantErr: true},       // non-numeric tenant
		{in: "tenants/0/devices/dev/up/telemetry", wantErr: true},         // tenant_id ≤ 0
		{in: "tenants/-1/devices/dev/up/telemetry", wantErr: true},        // negative
		{in: "tenants/1/devices/dev/sideways/telemetry", wantErr: true},   // bad direction
		{in: "tenants/1/foo/dev/up/telemetry", wantErr: true},             // bad layout
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			p, err := Parse(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", p)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.IsLegacy != tc.legacy {
				t.Errorf("IsLegacy = %v, want %v", p.IsLegacy, tc.legacy)
			}
			if !tc.legacy {
				if p.TenantID != tc.tenantID {
					t.Errorf("TenantID = %d, want %d", p.TenantID, tc.tenantID)
				}
				if p.DeviceID != tc.deviceID {
					t.Errorf("DeviceID = %q, want %q", p.DeviceID, tc.deviceID)
				}
				if p.Direction != tc.direction {
					t.Errorf("Direction = %q, want %q", p.Direction, tc.direction)
				}
				if p.Stream != tc.stream {
					t.Errorf("Stream = %q, want %q", p.Stream, tc.stream)
				}
			}
		})
	}
}

func TestLegacyDeviceID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"shellies/abc/emeter", "abc"},
		{"tele/dev1/STATE", "dev1"},
		{"single", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := LegacyDeviceID(tc.in)
		if got != tc.want {
			t.Errorf("LegacyDeviceID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
