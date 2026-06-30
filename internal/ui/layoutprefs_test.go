package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

func TestLayoutPreferencesSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	prefs := LayoutPreferences{Hosts: []string{"alpha", "beta", "gamma"}, Page: 1}
	if err := SaveLayoutPreferences(prefs); err != nil {
		t.Fatalf("SaveLayoutPreferences: %v", err)
	}

	loaded, err := LoadLayoutPreferences()
	if err != nil {
		t.Fatalf("LoadLayoutPreferences: %v", err)
	}
	if len(loaded.Hosts) != 3 || loaded.Hosts[0] != "alpha" || loaded.Hosts[2] != "gamma" {
		t.Fatalf("loaded hosts = %+v", loaded.Hosts)
	}
	if loaded.Page != 1 {
		t.Fatalf("loaded page = %d, want 1", loaded.Page)
	}
}

func TestLoadLayoutPreferencesMissingFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	prefs, err := LoadLayoutPreferences()
	if err != nil {
		t.Fatalf("LoadLayoutPreferences on missing file: %v", err)
	}
	if len(prefs.Hosts) != 0 {
		t.Fatalf("expected empty prefs, got %+v", prefs)
	}
}

func TestRestoreModelFromLayout(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	allHosts := []internal.SSHHost{
		{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"},
	}

	// No saved layout yet: restore should decline.
	if _, ok := RestoreModelFromLayout(allHosts, time.Second); ok {
		t.Fatalf("expected no restore without a saved layout")
	}

	// Save a layout that references one host that no longer exists ("ghost").
	if err := SaveLayoutPreferences(LayoutPreferences{Hosts: []string{"gamma", "ghost", "alpha"}, Page: 2}); err != nil {
		t.Fatalf("SaveLayoutPreferences: %v", err)
	}

	m, ok := RestoreModelFromLayout(allHosts, time.Second)
	if !ok {
		t.Fatalf("expected restore to succeed")
	}
	if m.screen != ScreenConnecting {
		t.Fatalf("restored screen = %v, want ScreenConnecting", m.screen)
	}
	if m.postConnectScreen != ScreenQuad {
		t.Fatalf("postConnectScreen = %v, want ScreenQuad", m.postConnectScreen)
	}
	if m.quadPage != 2 {
		t.Fatalf("quadPage = %d, want 2", m.quadPage)
	}
	// "ghost" is dropped; order of the survivors is preserved.
	if len(m.selectedHosts) != 2 || m.selectedHosts[0].Name != "gamma" || m.selectedHosts[1].Name != "alpha" {
		t.Fatalf("selectedHosts = %+v", m.selectedHosts)
	}
}

func TestSaveQuadLayoutStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := &Model{}
	m.saveQuadLayout()
	if m.quadStatus != "nothing to save" {
		t.Fatalf("empty save status = %q", m.quadStatus)
	}

	m.selectedHosts = []internal.SSHHost{{Name: "alpha"}, {Name: "beta"}}
	m.quadPage = 1
	m.saveQuadLayout()
	if !strings.HasPrefix(m.quadStatus, "✓") {
		t.Fatalf("save status = %q, want a ✓ confirmation", m.quadStatus)
	}

	loaded, err := LoadLayoutPreferences()
	if err != nil {
		t.Fatalf("LoadLayoutPreferences: %v", err)
	}
	if len(loaded.Hosts) != 2 || loaded.Page != 1 {
		t.Fatalf("persisted layout = %+v", loaded)
	}
}
