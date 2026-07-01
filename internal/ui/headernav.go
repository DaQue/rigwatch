package ui

import "strings"

// headerAction is one focusable item in the grid's header toolbar. Enter on a
// focused item runs apply (the same effect as its shortcut key).
type headerAction struct {
	key   string
	label string
	apply func(m *Model)
}

// quadHeaderActions is the toolbar shown in the grid/dual header, in Tab order.
// Quit is intentionally excluded so Enter can't quit by accident.
func quadHeaderActions() []headerAction {
	return []headerAction{
		{"v", "modes", func(m *Model) { m.openModeMenu() }},
		{"t", "dashboard", func(m *Model) { m.screen = ScreenDashboard }},
		{"c", "add hosts", func(m *Model) { m.screen = ScreenHostList; m.updateListSelection() }},
		{"w", "save layout", func(m *Model) { m.saveQuadLayout() }},
		{"?", "help", func(m *Model) { m.helpVisible = true }},
	}
}

// advanceGridFocus moves the Tab focus ring for the grid: through the visible
// panes, then off the last pane onto the header toolbar items one at a time,
// then back to the panes. headerFocus == -1 means a pane is focused.
func (m *Model) advanceGridFocus(dir int) {
	items := len(quadHeaderActions())
	count := m.visibleQuadHostCount()

	if m.headerFocus >= 0 {
		next := m.headerFocus + dir
		switch {
		case next < 0:
			// Back onto the last pane.
			m.headerFocus = -1
			m.quadFocus = max(0, count-1)
			m.armFocusHighlight()
		case next >= items:
			// Wrap back to the first pane.
			m.headerFocus = -1
			m.quadFocus = 0
			m.armFocusHighlight()
		default:
			m.headerFocus = next
		}
		return
	}

	// Currently on the panes.
	if dir > 0 {
		if count <= 1 || m.quadFocus >= count-1 {
			m.stepOntoHeader(0) // off the last pane onto the first toolbar item
		} else {
			m.moveQuadFocus(1)
			m.armFocusHighlight()
		}
	} else {
		if count <= 1 || m.quadFocus == 0 {
			m.stepOntoHeader(items - 1) // off the first pane onto the last item
		} else {
			m.moveQuadFocus(-1)
			m.armFocusHighlight()
		}
	}
}

func (m *Model) stepOntoHeader(idx int) {
	m.headerFocus = idx
	m.focusFramesLeft = 0 // no pane is focused now; clear the pane highlight
}

// activateHeaderItem runs the focused toolbar item and returns focus to the panes.
func (m *Model) activateHeaderItem() {
	actions := quadHeaderActions()
	if m.headerFocus < 0 || m.headerFocus >= len(actions) {
		return
	}
	idx := m.headerFocus
	m.headerFocus = -1
	actions[idx].apply(m)
}

// resetGridFocus puts focus back on the panes (used when the page or mode changes).
func (m *Model) resetGridFocus() {
	m.headerFocus = -1
}

// renderQuadToolbar renders the header actions, highlighting the focused one.
func (m Model) renderQuadToolbar() string {
	actions := quadHeaderActions()
	parts := make([]string, len(actions))
	for i, a := range actions {
		text := a.key + " " + a.label
		if m.headerFocus == i {
			parts[i] = accentStyle.Render("‹" + text + "›")
		} else {
			parts[i] = text
		}
	}
	return strings.Join(parts, "  •  ")
}
