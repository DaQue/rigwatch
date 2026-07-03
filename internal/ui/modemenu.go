package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// displayMode is a selectable view layout. Single and Overview map straight to a
// screen; Dual and Grid share the grid screen, differing only in tiles-per-page.
type displayMode int

const (
	modeSingle displayMode = iota
	modeDual
	modeGrid
	modeOverview
)

type modeOption struct {
	mode  displayMode
	label string
	desc  string
}

func modeOptions() []modeOption {
	return []modeOption{
		{modeSingle, "Single host", "one host, full detail"},
		{modeDual, "Dual pane", "two hosts side by side"},
		{modeGrid, "Grid (4-up)", "up to four host tiles"},
		{modeOverview, "Overview", "compact cards, all hosts"},
	}
}

// currentDisplayMode derives the active mode from the screen + grid page size, so
// the picker can preselect it.
func (m Model) currentDisplayMode() displayMode {
	switch m.screen {
	case ScreenOverview:
		return modeOverview
	case ScreenQuad:
		if m.gridTilesPerPage <= 2 {
			return modeDual
		}
		return modeGrid
	default:
		return modeSingle
	}
}

// openModeMenu shows the picker, preselecting the current mode.
func (m *Model) openModeMenu() {
	current := m.currentDisplayMode()
	m.modeMenuIdx = 0
	for i, opt := range modeOptions() {
		if opt.mode == current {
			m.modeMenuIdx = i
			break
		}
	}
	m.modeMenuOpen = true
}

// updateModeMenu drives the modal while it's open.
func (m Model) updateModeMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	opts := modeOptions()
	switch msg.String() {
	case "esc", "v":
		m.modeMenuOpen = false
	case "up", "k":
		m.modeMenuIdx = (m.modeMenuIdx - 1 + len(opts)) % len(opts)
	case "down", "j":
		m.modeMenuIdx = (m.modeMenuIdx + 1) % len(opts)
	case "enter", " ":
		m.modeMenuOpen = false
		m.applyDisplayMode(opts[m.modeMenuIdx].mode)
	}
	return m, nil
}

// applyDisplayMode switches to the chosen layout.
func (m *Model) applyDisplayMode(mode displayMode) {
	switch mode {
	case modeSingle:
		m.screen = ScreenDashboard
	case modeDual:
		m.screen = ScreenQuad
		m.gridTilesPerPage = 2
		m.quadPage = 0
		m.clampQuadFocus()
		m.resetGridFocus()
	case modeGrid:
		m.screen = ScreenQuad
		m.gridTilesPerPage = quadPageSize
		m.quadPage = 0
		m.clampQuadFocus()
		m.resetGridFocus()
	case modeOverview:
		m.screen = ScreenOverview
	}
}

func (m Model) renderModeMenu() string {
	var b strings.Builder
	b.WriteString(renderHeroHeader("RIGWATCH // DISPLAY MODE", "↑/↓ select  •  enter apply  •  esc cancel", m.width, m.animationFrame))
	b.WriteString("\n\n")

	var body strings.Builder
	for i, opt := range modeOptions() {
		pointer := "  "
		label := panelTextStyle.Render(fmt.Sprintf("%-14s", opt.label))
		if i == m.modeMenuIdx {
			pointer = accentStyle.Render("▸ ")
			label = accentStyle.Render(fmt.Sprintf("%-14s", opt.label))
		}
		if i > 0 {
			body.WriteString("\n")
		}
		body.WriteString(fmt.Sprintf("%s%s %s", pointer, label, mutedStyle.Render(opt.desc)))
	}

	width := clampInt(m.width-4, 40, 60)
	b.WriteString(renderPanel("DISPLAY MODE", body.String(), width))

	out := b.String()
	if m.height > 0 {
		out = clampToHeight(out, m.height)
	}
	return out
}
