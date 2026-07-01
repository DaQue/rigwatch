package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	in := DefaultSettings()
	in.Interval = 3.5
	in.DefaultTheme = "nord"
	in.Thresholds.DiskPct = Threshold{Warn: 80, Crit: 90}
	in.ShowExtendedPanels = false

	if err := SaveSettings(in); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.Interval != 3.5 {
		t.Fatalf("interval = %v, want 3.5", loaded.Interval)
	}
	if loaded.DefaultTheme != "nord" {
		t.Fatalf("theme = %q, want nord", loaded.DefaultTheme)
	}
	if loaded.Thresholds.DiskPct != (Threshold{Warn: 80, Crit: 90}) {
		t.Fatalf("disk threshold = %+v", loaded.Thresholds.DiskPct)
	}
	if loaded.ShowExtendedPanels {
		t.Fatalf("ShowExtendedPanels = true, want false")
	}
	if !loaded.CheckForUpdates {
		t.Fatalf("CheckForUpdates = false, want true")
	}
	if loaded.UpdateCheckIntervalHours != 24 {
		t.Fatalf("UpdateCheckIntervalHours = %d, want 24", loaded.UpdateCheckIntervalHours)
	}

	path, err := settingsPath()
	if err != nil {
		t.Fatalf("settingsPath: %v", err)
	}
	if filepath.Base(path) != "settings.json" {
		t.Fatalf("settings filename = %q, want settings.json", filepath.Base(path))
	}
}

func TestLoadSettingsMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings on missing file: %v", err)
	}
	want := DefaultSettings()
	if s.DefaultTheme != want.DefaultTheme {
		t.Fatalf("theme = %q, want %q", s.DefaultTheme, want.DefaultTheme)
	}
	// Temp threshold must match the previously hardcoded 70/85 coloring.
	if s.Thresholds.TempC != (Threshold{Warn: 70, Crit: 85}) {
		t.Fatalf("default temp threshold = %+v, want 70/85", s.Thresholds.TempC)
	}
	if !s.ShowExtendedPanels {
		t.Fatalf("ShowExtendedPanels default = false, want true")
	}
	if !s.CheckForUpdates {
		t.Fatalf("CheckForUpdates default = false, want true")
	}
	if s.UpdateCheckIntervalHours != 24 {
		t.Fatalf("UpdateCheckIntervalHours default = %d, want 24", s.UpdateCheckIntervalHours)
	}
}

func TestLoadSettingsBackfillsPartialFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// A hand-written partial file: only the interval is set.
	path := filepath.Join(dir, "rigwatch", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"interval": 2}`), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Interval != 2 {
		t.Fatalf("interval = %v, want 2", s.Interval)
	}
	// Missing fields backfilled from defaults.
	if s.DefaultTheme != defaultThemeName {
		t.Fatalf("theme = %q, want %q", s.DefaultTheme, defaultThemeName)
	}
	if s.Thresholds.DiskPct != (Threshold{Warn: 85, Crit: 95}) {
		t.Fatalf("disk threshold backfill = %+v, want 85/95", s.Thresholds.DiskPct)
	}
	if !s.CheckForUpdates {
		t.Fatalf("CheckForUpdates should backfill true for partial settings")
	}
	if s.UpdateCheckIntervalHours != 24 {
		t.Fatalf("UpdateCheckIntervalHours = %d, want 24", s.UpdateCheckIntervalHours)
	}
}
