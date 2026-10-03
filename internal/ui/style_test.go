package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func withStyle(t *testing.T, name string) {
	t.Helper()
	activeStyle = StyleByName(name)
	t.Cleanup(func() { activeStyle = classicStyle() })
}

func TestStyleRegistry(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range BuiltInStyles() {
		if s.Name == "" || seen[s.Name] {
			t.Fatalf("style name %q empty or duplicated", s.Name)
		}
		seen[s.Name] = true
		if s.Panel == nil || s.Hero == nil || s.Bar == nil {
			t.Fatalf("style %q has a nil renderer", s.Name)
		}
	}
	if StyleByName("nope").Name != styleClassic {
		t.Fatal("unknown style should fall back to classic")
	}
	if nextStyleName(styleClassic, 1) != styleDrift || nextStyleName(styleDrift, 1) != styleClassic {
		t.Fatal("style cycling should wrap")
	}
	if ThemeByName(marThemeName).Name != marThemeName {
		t.Fatal("mar theme missing")
	}
}

func TestSettingsNormalizeStyle(t *testing.T) {
	s := DefaultSettings()
	s.Style = "bogus"
	s.normalize()
	if s.Style != styleClassic {
		t.Fatalf("style = %q, want classic", s.Style)
	}
}

func TestCycleStyleSavesAndSwapsPalette(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := InitialModel(nil, time.Second)
	m.settings = DefaultSettings()

	m.cycleStyle()
	if m.settings.Style != styleDrift || m.settings.DefaultTheme != marThemeName {
		t.Fatalf("got %q/%q, want drift/mar", m.settings.Style, m.settings.DefaultTheme)
	}
	if loaded, err := LoadSettings(); err != nil || loaded.Style != styleDrift {
		t.Fatalf("style not persisted: %v %q", err, loaded.Style)
	}
	if m.styleToast == "" {
		t.Fatal("no confirmation toast")
	}

	m.cycleStyle()
	if m.settings.Style != styleClassic || m.settings.DefaultTheme != defaultThemeName {
		t.Fatalf("got %q/%q, want classic/rigwatch", m.settings.Style, m.settings.DefaultTheme)
	}

	// A palette the user chose themselves is left alone.
	m.settings.DefaultTheme = "nord"
	m.cycleStyle()
	if m.settings.DefaultTheme != "nord" {
		t.Fatalf("custom palette overwritten: %q", m.settings.DefaultTheme)
	}
}

func TestDriftPanelHasNoBoxAndMatchesClassicSize(t *testing.T) {
	body := "line one\nline two that is rather long and will need truncating to fit the panel width"
	for _, sev := range []Severity{SevOK, SevWarn, SevCrit} {
		classic := classicPanel("CPU", body, 40, sev)
		drift := driftPanel("CPU", body, 40, sev)
		if strings.ContainsAny(drift, "╭╮╰╯│") {
			t.Fatalf("drift panel drew a box:\n%s", drift)
		}
		cl, dl := strings.Split(classic, "\n"), strings.Split(drift, "\n")
		if len(cl) != len(dl) {
			t.Fatalf("height %d != classic %d", len(dl), len(cl))
		}
		for i, line := range dl {
			if w := lipgloss.Width(line); w != 40 {
				t.Fatalf("line %d is %d cols, want 40: %q", i, w, line)
			}
		}
	}
}

func TestDriftBarWidthAndThresholdTick(t *testing.T) {
	withStyle(t, styleDrift)
	bar := renderNeonProgressBarSev(20, 30, Threshold{Warn: 85, Crit: 95})
	if w := lipgloss.Width(bar); w != 30 {
		t.Fatalf("bar width %d, want 30", w)
	}
	if !strings.Contains(bar, driftTick) {
		t.Fatal("expected a warn tick on the empty track")
	}
	if strings.Contains(renderNeonProgressBarSev(100, 30, Threshold{Warn: 85}), driftEmpty) {
		t.Fatal("a full bar should have no empty cells")
	}
}

func TestDriftHeroKeepsClassicLineCountAndShowsToast(t *testing.T) {
	activeToast = "style: drift"
	t.Cleanup(func() { activeToast = "" })
	d := driftHero("RIGWATCH // HELP", "sub", 80, 3)
	c := classicHero("RIGWATCH // HELP", "sub", 80, 3)
	if strings.Count(d, "\n") != strings.Count(c, "\n") {
		t.Fatalf("hero heights differ: %d vs %d", strings.Count(d, "\n"), strings.Count(c, "\n"))
	}
	if !strings.Contains(d, "style: drift") {
		t.Fatal("toast missing from header")
	}
}

func TestSettingsFormStyleSaves(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := InitialModel(nil, time.Second)
	m.settings = DefaultSettings()
	(&m).openSettings()
	m.settingsForm.styleIdx = 1
	nm, _ := m.saveSettingsForm()
	if nm.(Model).settings.Style != styleDrift {
		t.Fatal("style not applied from settings form")
	}
}
