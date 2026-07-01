package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func gridModel(t *testing.T, n int) Model {
	t.Helper()
	names := make([]string, n)
	for i := range names {
		names[i] = string(rune('a' + i))
	}
	hosts := testHosts(names...)
	m := InitialModelWithHosts(hosts, hosts, 0)
	m.screen = ScreenQuad
	m.gridTilesPerPage = quadPageSize
	m.width = 140
	m.height = 40
	for _, h := range hosts {
		m.sysInfos[h.Name] = sampleLargeSystemInfo()
	}
	return m
}

func TestTabArmsFocusAndAdvances(t *testing.T) {
	m := gridModel(t, 4)
	m.quadFocus = 0
	m.focusFramesLeft = 0

	updated, _ := m.Update(key("tab"))
	m = updated.(Model)
	if m.quadFocus != 1 {
		t.Fatalf("tab should advance focus to 1, got %d", m.quadFocus)
	}
	if m.focusFramesLeft <= 0 {
		t.Fatalf("tab should arm the focus highlight, got %d", m.focusFramesLeft)
	}
	if m.modeMenuOpen {
		t.Fatalf("tab on a non-last pane should not open the menu")
	}
}

func TestFocusHighlightFadesOnTick(t *testing.T) {
	m := gridModel(t, 4)
	(&m).armFocusHighlight()
	start := m.focusFramesLeft
	if start <= 0 {
		t.Fatalf("armFocusHighlight should set frames > 0")
	}

	// Each animation tick decrements; after focusHoldFrames ticks it reaches 0.
	for i := 0; i < focusHoldFrames; i++ {
		updated, _ := m.Update(AnimationTickMsg(time.Now()))
		m = updated.(Model)
	}
	if m.focusFramesLeft != 0 {
		t.Fatalf("focus should fade to 0 after %d ticks, got %d", focusHoldFrames, m.focusFramesLeft)
	}
}

func TestTabStepsOffLastPaneOntoHeader(t *testing.T) {
	m := gridModel(t, 4)
	m.quadFocus = 3 // last pane on a 4-up page

	updated, _ := m.Update(key("tab"))
	m = updated.(Model)
	if m.modeMenuOpen {
		t.Fatalf("tab must not open the mode menu / close the view")
	}
	if m.headerFocus != 0 {
		t.Fatalf("tab off the last pane should focus the first header item, got %d", m.headerFocus)
	}
	if m.focusFramesLeft != 0 {
		t.Fatalf("pane highlight should clear once focus is on the header")
	}
}

func TestHeaderRingCyclesThenReturnsToPanes(t *testing.T) {
	m := gridModel(t, 2)
	m.gridTilesPerPage = 2
	m.quadFocus = 1 // last pane
	m.headerFocus = -1
	items := len(quadHeaderActions())

	// Step onto the header, then through every item.
	for i := 0; i < items; i++ {
		updated, _ := m.Update(key("tab"))
		m = updated.(Model)
		if m.headerFocus != i {
			t.Fatalf("tab %d: headerFocus = %d, want %d", i, m.headerFocus, i)
		}
	}
	// One more Tab wraps back to the panes.
	updated, _ := m.Update(key("tab"))
	m = updated.(Model)
	if m.headerFocus != -1 {
		t.Fatalf("tab past the last header item should return to the panes, got headerFocus %d", m.headerFocus)
	}
	if m.quadFocus != 0 {
		t.Fatalf("returning to panes should focus pane 0, got %d", m.quadFocus)
	}
}

func TestShiftTabFromFirstPaneStepsToLastHeaderItem(t *testing.T) {
	m := gridModel(t, 4)
	m.quadFocus = 0

	updated, _ := m.Update(key("shift+tab"))
	m = updated.(Model)
	if m.modeMenuOpen {
		t.Fatalf("shift+tab must not open the mode menu")
	}
	if want := len(quadHeaderActions()) - 1; m.headerFocus != want {
		t.Fatalf("shift+tab off pane 0 should focus the last header item %d, got %d", want, m.headerFocus)
	}
}

func TestEnterActivatesFocusedHeaderItem(t *testing.T) {
	m := gridModel(t, 2)
	// Focus the "modes" header item (index 0) and press enter.
	m.headerFocus = 0
	updated, _ := m.Update(key("enter"))
	m = updated.(Model)
	if !m.modeMenuOpen {
		t.Fatalf("enter on the 'modes' header item should open the mode menu")
	}
	if m.headerFocus != -1 {
		t.Fatalf("header focus should return to the panes after activating, got %d", m.headerFocus)
	}
}

func TestRenderQuadOmitsFocusWhenFaded(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := gridModel(t, 2)
	m.themePrefs = ThemePreferences{Hosts: map[string]string{"a": "rigwatch", "b": "rigwatch"}}
	m.quadFocus = 1

	// Faded: no focus label, no FocusColor border on the focused pane.
	m.focusFramesLeft = 0
	faded := m.renderQuad()
	if strings.Contains(faded, "[rigwatch]") {
		t.Fatalf("faded focus should not render the [theme] label:\n%s", faded)
	}

	// Armed: the focused pane shows its label.
	m.focusFramesLeft = focusHoldFrames
	armed := m.renderQuad()
	if !strings.Contains(armed, "[rigwatch]") {
		t.Fatalf("armed focus should render the [theme] label:\n%s", armed)
	}
}
