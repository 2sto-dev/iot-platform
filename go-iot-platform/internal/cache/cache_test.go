package cache

import "testing"

func TestParseTenantTag(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "unassigned"},
		{-1, "unassigned"},
		{1, "1"},
		{42, "42"},
		{9999, "9999"},
	}
	for _, tc := range cases {
		got := ParseTenantTag(tc.in)
		if got != tc.want {
			t.Errorf("ParseTenantTag(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
