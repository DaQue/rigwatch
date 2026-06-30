package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestHelpOverlayToggle(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.screen = ScreenDashboard

	// "?" opens help.
	updated, _ := m.Update(key("?"))
	m = updated.(Model)
	if !m.helpVisible {
		t.Fatalf("expected help to open on '?'")
	}

	// While open, a normal key is swallowed (screen unchanged, help stays).
	updated, _ = m.Update(key("g"))
	m = updated.(Model)
	if !m.helpVisible {
		t.Fatalf("help should stay open and swallow non-dismiss keys")
	}
	if m.screen != ScreenDashboard {
		t.Fatalf("screen changed under help overlay: %v", m.screen)
	}

	// Esc closes it.
	updated, _ = m.Update(key("esc"))
	m = updated.(Model)
	if m.helpVisible {
		t.Fatalf("expected help to close on esc")
	}
}

func TestHelpGroupsNonEmpty(t *testing.T) {
	groups := helpGroups()
	if len(groups) == 0 {
		t.Fatal("helpGroups is empty")
	}
	for _, g := range groups {
		if g.title == "" || len(g.entries) == 0 {
			t.Fatalf("group %q has no entries", g.title)
		}
	}
}
