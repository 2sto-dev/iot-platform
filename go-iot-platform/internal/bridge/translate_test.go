package bridge

import (
	"testing"
)

func TestParseLegacy(t *testing.T) {
	cases := []struct {
		topic       string
		wantSerial  string
		wantStream  string
		wantOK      bool
	}{
		{"shellies/SHELF001/emeter", "SHELF001", "emeter", true},
		{"shellies/SHELF001/relay/0", "SHELF001", "relay", true},
		{"shellies/SHELF001", "SHELF001", "status", true},
		{"tele/NOUS001/SENSOR", "NOUS001", "sensor", true},
		{"tele/NOUS001/LWT", "NOUS001", "lwt", true},
		{"tele/NOUS001", "NOUS001", "tele", true},
		{"zigbee2mqtt/ZIGB001", "ZIGB001", "zigbee", true},
		{"tenants/1/devices/X/up/power", "", "", false},
		{"unknown/topic", "", "", false},
		{"shellies/", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		serial, stream, ok := ParseLegacy(tc.topic)
		if ok != tc.wantOK {
			t.Errorf("ParseLegacy(%q) ok=%v want %v", tc.topic, ok, tc.wantOK)
			continue
		}
		if ok {
			if serial != tc.wantSerial {
				t.Errorf("ParseLegacy(%q) serial=%q want %q", tc.topic, serial, tc.wantSerial)
			}
			if stream != tc.wantStream {
				t.Errorf("ParseLegacy(%q) stream=%q want %q", tc.topic, stream, tc.wantStream)
			}
		}
	}
}

func TestNewTopic(t *testing.T) {
	got := NewTopic(5, "SHELF001", "emeter")
	want := "tenants/5/devices/SHELF001/up/emeter"
	if got != want {
		t.Errorf("NewTopic(5, SHELF001, emeter) = %q; want %q", got, want)
	}
}
