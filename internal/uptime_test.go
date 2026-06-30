package internal

import (
	"testing"
	"time"
)

func TestParseUptime(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"350735.47 234388.90", time.Duration(350735.47 * float64(time.Second))},
		{"60.00 0.00", time.Minute},
		{"  90 0\n", 90 * time.Second},
		{"", 0},
		{"garbage", 0},
		{"-5 0", 0},
	}
	for _, c := range cases {
		if got := parseUptime(c.in); got != c.want {
			t.Errorf("parseUptime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
