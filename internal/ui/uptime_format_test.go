package ui

import (
	"testing"
	"time"
)

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0m"},
		{-1, "0m"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{25 * time.Hour, "1d 1h"},
		{(4*24 + 3) * time.Hour, "4d 3h"},
	}
	for _, c := range cases {
		if got := formatUptime(c.in); got != c.want {
			t.Errorf("formatUptime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
