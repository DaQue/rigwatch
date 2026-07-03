package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

func TestPaneLayoutDualPaging(t *testing.T) {
	// maxTiles=2 must page two hosts at a time regardless of how many fit.
	layout := paneLayout(220, 70, 5, 0, 2)
	if layout.PageSize() != 2 {
		t.Fatalf("dual page size = %d, want 2", layout.PageSize())
	}
	if layout.Pages != 3 {
		t.Fatalf("pages = %d, want 3 for 5 hosts at 2/page", layout.Pages)
	}
	if layout.Columns != 2 || layout.Rows != 1 {
		t.Fatalf("wide dual layout = %dx%d, want 2x1", layout.Columns, layout.Rows)
	}
}

func TestApplyDisplayMode(t *testing.T) {
	cases := []struct {
		mode       displayMode
		wantScreen Screen
		wantTiles  int // 0 = don't check
	}{
		{modeSingle, ScreenDashboard, 0},
		{modeDual, ScreenQuad, 2},
		{modeGrid, ScreenQuad, quadPageSize},
		{modeOverview, ScreenOverview, 0},
	}
	for _, c := range cases {
		m := InitialModel(nil, time.Second)
		(&m).applyDisplayMode(c.mode)
		if m.screen != c.wantScreen {
			t.Errorf("mode %d: screen = %v, want %v", c.mode, m.screen, c.wantScreen)
		}
		if c.wantTiles != 0 && m.gridTilesPerPage != c.wantTiles {
			t.Errorf("mode %d: tiles = %d, want %d", c.mode, m.gridTilesPerPage, c.wantTiles)
		}
	}
}

func TestCurrentDisplayModeRoundTrips(t *testing.T) {
	for _, mode := range []displayMode{modeSingle, modeDual, modeGrid, modeOverview} {
		m := InitialModel(nil, time.Second)
		(&m).applyDisplayMode(mode)
		if got := m.currentDisplayMode(); got != mode {
			t.Errorf("applied %d but currentDisplayMode = %d", mode, got)
		}
	}
}

func TestModeMenuOpenSelectApply(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.screen = ScreenDashboard

	// v opens the picker, preselecting Single.
	updated, _ := m.Update(key("v"))
	m = updated.(Model)
	if !m.modeMenuOpen {
		t.Fatalf("v should open the mode menu")
	}
	if m.modeMenuIdx != int(modeSingle) {
		t.Fatalf("expected Single preselected, got idx %d", m.modeMenuIdx)
	}

	// Down to Dual, then enter applies it.
	updated, _ = m.Update(key("down"))
	m = updated.(Model)
	updated, _ = m.Update(key("enter"))
	m = updated.(Model)
	if m.modeMenuOpen {
		t.Fatalf("enter should close the menu")
	}
	if m.screen != ScreenQuad || m.gridTilesPerPage != 2 {
		t.Fatalf("expected dual grid, got screen=%v tiles=%d", m.screen, m.gridTilesPerPage)
	}
}

func TestModeMenuEscCancels(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.screen = ScreenOverview
	(&m).openModeMenu()

	updated, _ := m.Update(key("esc"))
	m = updated.(Model)
	if m.modeMenuOpen {
		t.Fatalf("esc should close the menu")
	}
	if m.screen != ScreenOverview {
		t.Fatalf("esc should not change the screen, got %v", m.screen)
	}
}

func TestLayoutPreferencesModeRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := SaveLayoutPreferences(LayoutPreferences{Hosts: []string{"a", "b"}, Page: 0, TilesPerPage: 2}); err != nil {
		t.Fatalf("SaveLayoutPreferences: %v", err)
	}

	allHosts := []internal.SSHHost{{Name: "a"}, {Name: "b"}}
	m, ok := RestoreModelFromLayout(allHosts, time.Second)
	if !ok {
		t.Fatalf("expected restore to succeed")
	}
	if m.gridTilesPerPage != 2 {
		t.Fatalf("restored tiles = %d, want 2 (dual)", m.gridTilesPerPage)
	}
}
