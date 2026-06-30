package ui

import (
	"testing"
	"time"
)

func themeIndex(name string) int {
	for i, n := range ThemeNames() {
		if n == name {
			return i
		}
	}
	return 0
}

func TestSettingsFormSaveAppliesAndPersists(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := InitialModel(nil, 5*time.Second)
	m.settings = DefaultSettings()

	(&m).openSettings()
	if m.screen != ScreenSettings {
		t.Fatalf("openSettings: screen = %v, want ScreenSettings", m.screen)
	}

	m.settingsForm.inputs[0].SetValue("2.5") // interval
	m.settingsForm.inputs[1].SetValue("60")  // CPU warn
	m.settingsForm.inputs[2].SetValue("80")  // CPU crit
	m.settingsForm.themeIdx = themeIndex("nord")

	updated, _ := m.saveSettingsForm()
	nm := updated.(Model)

	if nm.settingsForm != nil {
		t.Errorf("settingsForm not cleared after save")
	}
	if nm.screen != ScreenHostList {
		t.Errorf("screen after save = %v, want ScreenHostList", nm.screen)
	}
	if nm.settings.Interval != 2.5 {
		t.Errorf("interval = %v, want 2.5", nm.settings.Interval)
	}
	if nm.settings.DefaultTheme != "nord" {
		t.Errorf("theme = %q, want nord", nm.settings.DefaultTheme)
	}
	if nm.settings.Thresholds.CPUPct != (Threshold{Warn: 60, Crit: 80}) {
		t.Errorf("CPU threshold = %+v, want 60/80", nm.settings.Thresholds.CPUPct)
	}
	if nm.updateInterval != 2500*time.Millisecond {
		t.Errorf("updateInterval = %v, want 2.5s", nm.updateInterval)
	}

	// Persisted to disk and reloads.
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.Interval != 2.5 || loaded.DefaultTheme != "nord" {
		t.Errorf("persisted settings = %+v", loaded)
	}
}

func TestSettingsFormRejectsBadNumber(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := InitialModel(nil, 5*time.Second)
	m.settings = DefaultSettings()
	(&m).openSettings()
	m.settingsForm.inputs[0].SetValue("abc")

	updated, _ := m.saveSettingsForm()
	nm := updated.(Model)
	if nm.screen != ScreenSettings {
		t.Errorf("bad input should keep settings screen open, got %v", nm.screen)
	}
	if nm.settingsForm == nil || !nm.settingsForm.statusErr {
		t.Errorf("expected an error status on bad input")
	}
}

func TestParseOptionalFloat(t *testing.T) {
	if v, err := parseOptionalFloat(""); err != nil || v != 0 {
		t.Errorf("empty = (%v,%v), want (0,nil)", v, err)
	}
	if v, err := parseOptionalFloat(" 42 "); err != nil || v != 42 {
		t.Errorf("'42' = (%v,%v), want (42,nil)", v, err)
	}
	if _, err := parseOptionalFloat("-1"); err == nil {
		t.Errorf("negative should error")
	}
	if _, err := parseOptionalFloat("x"); err == nil {
		t.Errorf("non-number should error")
	}
}
