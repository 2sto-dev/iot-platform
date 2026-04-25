package influx

import "testing"

func TestRangeRegex(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"-5m", true},
		{"-1h", true},
		{"-30s", true},
		{"-7d", true},
		{"5m", true},
		{"", false},
		{"-5", false},
		{"5min", false},
		{"-5x", false},
		{"5m; drop table", false},
	}
	for _, tc := range cases {
		got := rangeRe.MatchString(tc.in)
		if got != tc.valid {
			t.Errorf("rangeRe.MatchString(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}
