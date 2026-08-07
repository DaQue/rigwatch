package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

func TestSettingsFormCyclesHeadlineMode(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := InitialModel(nil, 5*time.Second)
	m.settings = DefaultSettings()
	(&m).openSettings()

	// The form opens on the current mode, and focus 1 is the headline cycler.
	if got := HeadlineModes()[m.settingsForm.headlineIdx]; got != HeadlineLarge {
		t.Fatalf("form opened on %q, want %q", got, HeadlineLarge)
	}
	(&m).setSettingsFocus(1)
	updated, _ := m.updateSettings(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	if got := HeadlineModes()[m.settingsForm.headlineIdx]; got != HeadlineCompact {
		t.Fatalf("after → the mode is %q, want %q", got, HeadlineCompact)
	}

	saved, _ := m.saveSettingsForm()
	nm := saved.(Model)
	if nm.settings.HeadlineMode != HeadlineCompact {
		t.Fatalf("saved mode = %q, want %q", nm.settings.HeadlineMode, HeadlineCompact)
	}
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.HeadlineMode != HeadlineCompact {
		t.Fatalf("persisted mode = %q, want %q", loaded.HeadlineMode, HeadlineCompact)
	}
}

// The threshold inputs sit after both cycler rows; a stale offset would write
// the interval into a threshold.
func TestSettingsFormThresholdFocusOffsets(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := InitialModel(nil, 5*time.Second)
	m.settings = DefaultSettings()
	(&m).openSettings()

	if got := inputIndexFor(2); got != 0 {
		t.Fatalf("focus 2 maps to input %d, want 0 (interval)", got)
	}
	if got := inputIndexFor(3); got != 1 {
		t.Fatalf("focus 3 maps to input %d, want 1 (CPU warn)", got)
	}
	if got := m.settingsForm.focusCount(); got != settingsCyclerRows+len(m.settingsForm.inputs) {
		t.Fatalf("focusCount = %d, want %d", got, settingsCyclerRows+len(m.settingsForm.inputs))
	}
}
