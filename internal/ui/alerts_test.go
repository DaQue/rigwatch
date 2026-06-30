package ui

import (
	"testing"

	"github.com/allisonhere/rigwatch/internal"
)

func TestSeverityFor(t *testing.T) {
	th := Threshold{Warn: 70, Crit: 85}
	cases := []struct {
		value float64
		want  Severity
	}{
		{50, SevOK},
		{69.9, SevOK},
		{70, SevWarn},
		{84.9, SevWarn},
		{85, SevCrit},
		{99, SevCrit},
	}
	for _, c := range cases {
		if got := severityFor(c.value, th); got != c.want {
			t.Errorf("severityFor(%v) = %v, want %v", c.value, got, c.want)
		}
	}

	// A disabled (0/0) threshold never alerts, even at 100.
	if got := severityFor(100, Threshold{}); got != SevOK {
		t.Errorf("disabled threshold severityFor(100) = %v, want SevOK", got)
	}
}

func TestHostAlertsNilInfo(t *testing.T) {
	if a := hostAlerts(nil, DefaultThresholds()); a != nil {
		t.Fatalf("hostAlerts(nil) = %v, want nil", a)
	}
}

func TestHostAlertsClassifiesAndSorts(t *testing.T) {
	info := &internal.SystemInfo{
		CPU: internal.CPUInfo{UsagePercent: 90},                            // warn (85/95)
		RAM: internal.RAMInfo{Total: 16000, Used: 15500, UsagePercent: 96}, // crit (85/95)
		Disk: []internal.DiskInfo{
			{MountPoint: "/", UsagePercent: "50%"},     // ok
			{MountPoint: "/data", UsagePercent: "99%"}, // crit
		},
		Temps: []internal.TemperatureInfo{
			{Name: "CPU", Celsius: 60}, // ok
		},
		GPUs: []internal.GPUInfo{
			{Index: "0", Temperature: 92, Utilization: 100}, // temp crit (80/90); util off by default
		},
	}

	alerts := hostAlerts(info, DefaultThresholds())
	if len(alerts) == 0 {
		t.Fatal("expected alerts, got none")
	}

	// Critical alerts must come before warnings.
	seenWarn := false
	for _, a := range alerts {
		if a.Sev == SevWarn {
			seenWarn = true
		}
		if a.Sev == SevCrit && seenWarn {
			t.Fatalf("crit alert after warn — not sorted worst-first: %+v", alerts)
		}
	}

	// CPU should be the lone warning; RAM, /data, and GPU temp should be crit.
	var crit, warn int
	for _, a := range alerts {
		switch a.Sev {
		case SevCrit:
			crit++
		case SevWarn:
			warn++
		}
	}
	if warn != 1 {
		t.Errorf("warnings = %d, want 1 (CPU)", warn)
	}
	if crit != 3 {
		t.Errorf("criticals = %d, want 3 (RAM, /data disk, GPU temp)", crit)
	}

	if worstSeverity(info, DefaultThresholds()) != SevCrit {
		t.Errorf("worstSeverity = %v, want SevCrit", worstSeverity(info, DefaultThresholds()))
	}
}

func TestHostAlertsAllNominal(t *testing.T) {
	info := &internal.SystemInfo{
		CPU:   internal.CPUInfo{UsagePercent: 10},
		RAM:   internal.RAMInfo{Total: 16000, Used: 1600, UsagePercent: 10},
		Temps: []internal.TemperatureInfo{{Name: "CPU", Celsius: 45}},
	}
	if a := hostAlerts(info, DefaultThresholds()); len(a) != 0 {
		t.Fatalf("expected no alerts, got %+v", a)
	}
	if worstSeverity(info, DefaultThresholds()) != SevOK {
		t.Errorf("worstSeverity = %v, want SevOK", worstSeverity(info, DefaultThresholds()))
	}
}

func TestParsePercent(t *testing.T) {
	cases := map[string]struct {
		val float64
		ok  bool
	}{
		"73%":  {73, true},
		" 8% ": {8, true},
		"100":  {100, true},
		"":     {0, false},
		"n/a":  {0, false},
	}
	for in, want := range cases {
		got, ok := parsePercent(in)
		if ok != want.ok || (ok && got != want.val) {
			t.Errorf("parsePercent(%q) = (%v, %v), want (%v, %v)", in, got, ok, want.val, want.ok)
		}
	}
}
