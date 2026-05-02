package influx

import "testing"

func TestBucketConfigDefaults(t *testing.T) {
	cfg := BucketConfig{}
	cfg.applyDefaults()
	if cfg.Free != "iot-free" {
		t.Errorf("Free=%q want iot-free", cfg.Free)
	}
	if cfg.Pro != "iot-pro" {
		t.Errorf("Pro=%q want iot-pro", cfg.Pro)
	}
	if cfg.Enterprise != "iot-enterprise" {
		t.Errorf("Enterprise=%q want iot-enterprise", cfg.Enterprise)
	}
}

func TestBucketConfigForPlan(t *testing.T) {
	cfg := BucketConfig{Free: "iot-free", Pro: "iot-pro", Enterprise: "iot-enterprise"}
	cases := []struct {
		plan   string
		bucket string
	}{
		{"free", "iot-free"},
		{"pro", "iot-pro"},
		{"enterprise", "iot-enterprise"},
		{"", "iot-free"},
		{"unknown", "iot-free"},
	}
	for _, tc := range cases {
		got := cfg.ForPlan(tc.plan)
		if got != tc.bucket {
			t.Errorf("ForPlan(%q) = %q; want %q", tc.plan, got, tc.bucket)
		}
	}
}
