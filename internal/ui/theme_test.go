package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestThemeRegistryIncludesRigwatchAndUniqueNames(t *testing.T) {
	themes := BuiltInThemes()
	if len(themes) < 10 {
		t.Fatalf("theme count = %d, want at least 10", len(themes))
	}

	seen := make(map[string]bool)
	for _, theme := range themes {
		if theme.Name == "" {
			t.Fatalf("theme with empty name: %+v", theme)
		}
		if seen[theme.Name] {
			t.Fatalf("duplicate theme name %q", theme.Name)
		}
		seen[theme.Name] = true
	}

	rigwatch := ThemeByName("rigwatch")
	if rigwatch.Name != "rigwatch" {
		t.Fatalf("ThemeByName(rigwatch) = %q", rigwatch.Name)
	}
	if rigwatch.Ink != inkColor || rigwatch.Border != violetColor || rigwatch.Accent != cyanColor {
		t.Fatalf("rigwatch theme should preserve current palette: %+v", rigwatch)
	}
	if got := ThemeByName("missing").Name; got != "rigwatch" {
		t.Fatalf("unknown theme fallback = %q, want rigwatch", got)
	}
}

func TestThemePreferencesSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	prefs := ThemePreferences{Hosts: map[string]string{"box": "nord", "localhost": "amber-crt"}}
	if err := SaveThemePreferences(prefs); err != nil {
		t.Fatalf("SaveThemePreferences: %v", err)
	}

	loaded, err := LoadThemePreferences()
	if err != nil {
		t.Fatalf("LoadThemePreferences: %v", err)
	}
	if loaded.Hosts["box"] != "nord" || loaded.Hosts["localhost"] != "amber-crt" {
		t.Fatalf("loaded prefs = %+v", loaded)
	}

	path, err := themePreferencesPath()
	if err != nil {
		t.Fatalf("themePreferencesPath: %v", err)
	}
	if filepath.Base(path) != "themes.json" {
		t.Fatalf("preference filename = %q, want themes.json", filepath.Base(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected saved prefs at %s: %v", path, err)
	}
}

func TestThemePreferencesUnknownHostAndThemeFallbackToRigwatch(t *testing.T) {
	prefs := ThemePreferences{Hosts: map[string]string{"box": "not-a-theme"}}
	if got := prefs.ThemeNameForHost("missing"); got != "rigwatch" {
		t.Fatalf("missing host theme = %q, want rigwatch", got)
	}
	if got := prefs.ThemeNameForHost("box"); got != "rigwatch" {
		t.Fatalf("unknown saved theme fallback = %q, want rigwatch", got)
	}
}

func TestQuadThemeControlsFocusAndCycleFocusedHost(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := InitialModelWithHosts(testHosts("alpha", "beta"), testHosts("alpha", "beta"), 0)
	m.screen = ScreenQuad
	m.width = 140
	m.height = 40
	m.quadFocus = 0
	m.themePrefs = ThemePreferences{Hosts: map[string]string{"alpha": "rigwatch", "beta": "rigwatch"}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if m.quadFocus != 1 {
		t.Fatalf("quadFocus after tab = %d, want 1", m.quadFocus)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = updated.(Model)
	if m.themePrefs.Hosts["alpha"] != "rigwatch" {
		t.Fatalf("alpha theme changed unexpectedly: %+v", m.themePrefs.Hosts)
	}
	if m.themePrefs.Hosts["beta"] == "rigwatch" || m.themePrefs.Hosts["beta"] == "" {
		t.Fatalf("beta theme did not cycle: %+v", m.themePrefs.Hosts)
	}
	if !strings.Contains(m.quadStatus, "beta theme:") {
		t.Fatalf("theme cycle status = %q, want beta confirmation", m.quadStatus)
	}

	loaded, err := LoadThemePreferences()
	if err != nil {
		t.Fatalf("LoadThemePreferences: %v", err)
	}
	if loaded.Hosts["beta"] != m.themePrefs.Hosts["beta"] {
		t.Fatalf("saved beta theme = %q, want %q", loaded.Hosts["beta"], m.themePrefs.Hosts["beta"])
	}
}

func TestQuadThemeCycleReportsPersistenceFailure(t *testing.T) {
	configHome := filepath.Join(t.TempDir(), "config-file")
	if err := os.WriteFile(configHome, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("write config sentinel: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)

	m := InitialModelWithHosts(testHosts("alpha"), testHosts("alpha"), 0)
	m.screen = ScreenQuad
	m.width = 120
	m.height = 40
	m.themePrefs = ThemePreferences{Hosts: map[string]string{"alpha": "rigwatch"}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	m = updated.(Model)
	if !strings.HasPrefix(m.quadStatus, "theme save failed:") {
		t.Fatalf("quadStatus = %q, want save failure", m.quadStatus)
	}
}

func TestQuadRendersDifferentHostThemesAndFocusedThemeName(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := InitialModelWithHosts(testHosts("alpha", "beta"), testHosts("alpha", "beta"), 0)
	m.screen = ScreenQuad
	m.width = 140
	m.height = 40
	m.quadFocus = 1
	m.focusFramesLeft = focusHoldFrames // focus highlight is transient; arm it
	m.themePrefs = ThemePreferences{Hosts: map[string]string{"alpha": "rigwatch", "beta": "nord"}}
	m.sysInfos = map[string]*internal.SystemInfo{
		"alpha": sampleLargeSystemInfo(),
		"beta":  sampleLargeSystemInfo(),
	}

	got := m.renderQuad()
	if !strings.Contains(got, "◈ beta [nord]") {
		t.Fatalf("focused pane missing theme name:\n%s", got)
	}
	if colors := uniqueForegroundColorsForGlyph(got, "╭"); colors < 2 {
		t.Fatalf("expected multiple pane border colors for different host themes, got %d:\n%s", colors, got)
	}
}

func TestDashboardViewUsesCurrentHostTheme(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := InitialModelWithHosts(testHosts("alpha"), testHosts("alpha"), 0)
	m.screen = ScreenDashboard
	m.width = 120
	m.height = 40
	m.clients["alpha"] = &internal.SSHClient{}
	m.sysInfos["alpha"] = sampleLargeSystemInfo()
	m.themePrefs = ThemePreferences{Hosts: map[string]string{"alpha": "nord"}}

	got := m.View()
	if colors := uniqueForegroundColorsForGlyph(got, "╭"); colors == 0 {
		t.Fatalf("expected themed dashboard borders:\n%s", got)
	}
	if !strings.Contains(got, "\x1b[38;2;94;129;172m╭") {
		t.Fatalf("dashboard did not use nord border color:\n%s", got)
	}
}

func testHosts(names ...string) []internal.SSHHost {
	hosts := make([]internal.SSHHost, len(names))
	for i, name := range names {
		hosts[i] = internal.SSHHost{Name: name, Hostname: name, Port: "22"}
	}
	return hosts
}
