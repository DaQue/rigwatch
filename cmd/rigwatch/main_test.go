package main

import (
	"testing"
	"time"
)

func TestValidateInterval(t *testing.T) {
	tests := []struct {
		name    string
		seconds float64
		want    time.Duration
	}{
		{name: "rejects below minimum", seconds: 0.009, want: 0},
		{name: "accepts minimum", seconds: 0.01, want: 10 * time.Millisecond},
		{name: "accepts decimal seconds", seconds: 0.5, want: 500 * time.Millisecond},
		{name: "accepts normal seconds", seconds: 5, want: 5 * time.Second},
		{name: "accepts maximum", seconds: 3600, want: time.Hour},
		{name: "rejects above maximum", seconds: 3600.001, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateInterval(tt.seconds); got != tt.want {
				t.Fatalf("validateInterval(%v) = %v, want %v", tt.seconds, got, tt.want)
			}
		})
	}
}
