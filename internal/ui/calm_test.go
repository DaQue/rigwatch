package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

func luminance(hex string) float64 {
	return 0.2126*hexRed(hex) + 0.7152*hexGreen(hex) + 0.0722*hexBlue(hex)
}

func TestCalmThemeDimsTowardBackgroundAndKeepsHue(t *testing.T) {
	base := ThemeByName("rigwatch")
	calm := calmTheme(base)
	for name, pair := range map[string][2]string{
		"ink":    {string(base.Ink), string(calm.Ink)},
		"accent": {string(base.Accent), string(calm.Accent)},
		"border": {string(base.Border), string(calm.Border)},
	} {
		if luminance(pair[1]) >= luminance(pair[0]) {
			t.Errorf("%s got no dimmer: %s → %s", name, pair[0], pair[1])
		}
	}
	if calm.Name != base.Name {
		t.Errorf("calm theme renamed to %q; the name drives theme cycling", calm.Name)
	}
	if len(calm.CoolRamp) != len(base.CoolRamp) || calm.CoolRamp[2].red >= base.CoolRamp[2].red {
		t.Errorf("ramp not dimmed: %+v vs %+v", calm.CoolRamp, base.CoolRamp)
	}
	if got := blendHex("#ffffff", "#000000", 0.5); got != "#808080" {
		t.Errorf("blend = %s, want #808080", got)
	}
	if got := blendHex("red", "#000000", 0.5); got != "red" {
		t.Errorf("non-hex input should pass through, got %s", got)
	}
}

func TestCalmModeDimsOnlyHealthyHosts(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.settings = DefaultSettings()
	m.sysInfos["calm"] = &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 5}}
	m.sysInfos["hot"] = &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 99}}

	full := ThemeByName(m.settings.DefaultTheme).Accent
	if got := m.themeForHost("calm").Accent; got != full {
		t.Fatalf("calm mode off changed the theme: %s", got)
	}

	m.settings.CalmMode = true
	if got := m.themeForHost("calm").Accent; got == full {
		t.Errorf("healthy host not dimmed in calm mode")
	}
	if got := m.themeForHost("hot").Accent; got != full {
		t.Errorf("host with a critical alert was dimmed (%s); alerts must stay vivid", got)
	}
}

func TestSettingsFormCalmToggleSaves(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := InitialModel(nil, time.Second)
	m.settings = DefaultSettings()
	(&m).openSettings()
	m.settingsForm.calm = true
	nm, _ := m.saveSettingsForm()
	if !nm.(Model).settings.CalmMode {
		t.Fatal("calm mode not applied")
	}
	loaded, err := LoadSettings()
	if err != nil || !loaded.CalmMode {
		t.Fatalf("calm mode not persisted: %v %+v", err, loaded.CalmMode)
	}
}

func TestInsightWarningKeepsHostVividAndFlagsTile(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.settings = DefaultSettings()
	m.settings.CalmMode = true
	m.sysInfos["rig"] = &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 5}}
	if m.worstHostSeverity("rig") != SevOK {
		t.Fatal("baseline should be healthy")
	}
	m.metricHistories["rig"] = metricHistory{Insights: []Insight{{Key: "throttle:gpu0", Sev: SevWarn}}}
	if m.worstHostSeverity("rig") != SevWarn {
		t.Fatal("a warning insight should raise the host severity")
	}
	if m.themeForHost("rig").Accent != ThemeByName(m.settings.DefaultTheme).Accent {
		t.Fatal("host with a warning insight was dimmed")
	}
	m.metricHistories["rig"] = metricHistory{Insights: []Insight{{Key: "forecast:ram", Sev: SevOK}}}
	if m.worstHostSeverity("rig") != SevOK {
		t.Fatal("informational insights must not raise severity")
	}
}
