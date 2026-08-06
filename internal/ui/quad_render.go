package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

const quadPageSize = 4

func (m Model) renderQuad() string {
	var b strings.Builder

	total := len(m.selectedHosts)
	layout := paneLayout(m.width, m.height, total, m.quadPage, m.gridTilesPerPage)
	m.quadPage = clampInt(m.quadPage, 0, layout.Pages-1)
	m.quadFocus = clampInt(m.quadFocus, 0, max(0, layout.End-layout.Start-1))

	pageHint := ""
	if layout.Pages > 1 {
		pageHint = fmt.Sprintf("  •  page %d/%d  •  n/p page", m.quadPage+1, layout.Pages)
	}
	statusHint := ""
	if m.quadStatus != "" {
		statusHint = "  •  " + m.quadStatus
	}
	subtitle := fmt.Sprintf("v%s  •  refreshed %s  •  interval %s%s  •  %s  •  q quit%s",
		internal.ShortVersion(), time.Now().Format("15:04:05"), formatInterval(m.updateInterval), pageHint, m.renderQuadToolbar(), statusHint)
	gridTitle := "COMMAND CENTER // GRID"
	if m.gridTilesPerPage <= 2 {
		gridTitle = "COMMAND CENTER // DUAL"
	}
	b.WriteString(renderHeroHeader(gridTitle, subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")

	for row := 0; row < layout.Rows; row++ {
		cells := make([]string, 0, layout.Columns)
		for col := 0; col < layout.Columns; col++ {
			idx := layout.Start + row*layout.Columns + col
			if idx < layout.End {
				// The focus highlight is transient: only the focused pane shows it,
				// and only for a short window after the user moves focus.
				focused := idx-layout.Start == m.quadFocus && m.focusFramesLeft > 0
				cells = append(cells, m.renderQuadPanelWithFocus(m.selectedHosts[idx], layout.CellWidth, layout.BodyLines, focused))
			} else {
				cells = append(cells, renderEmptyQuadCell(layout.CellWidth, layout.BodyLines))
			}
		}
		b.WriteString(joinGridCells(cells))
		b.WriteString("\n")
	}

	return b.String()
}

func joinPaneRows(cells []string, columns int) string {
	if len(cells) == 0 {
		return ""
	}
	columns = max(1, columns)
	var b strings.Builder
	for i := 0; i < len(cells); i += columns {
		end := i + columns
		if end > len(cells) {
			end = len(cells)
		}
		b.WriteString(joinGridCells(cells[i:end]))
		b.WriteString("\n")
	}
	return b.String()
}

func joinGridCells(cells []string) string {
	if len(cells) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cells)*2-1)
	for i, cell := range cells {
		if i > 0 {
			parts = append(parts, "  ")
		}
		parts = append(parts, cell)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func renderEmptyQuadCell(width, maxBodyLines int) string {
	lines := fitLinesToPane(mutedStyle.Render("no host"), maxBodyLines)
	return renderPanel("◇", strings.Join(lines, "\n"), width)
}

func (m Model) renderQuadPanel(host internal.SSHHost, width, maxBodyLines int) string {
	return m.renderQuadPanelWithFocus(host, width, maxBodyLines, false)
}

func (m Model) renderQuadPanelWithFocus(host internal.SSHHost, width, maxBodyLines int, focused bool) string {
	return m.renderHostPane(host, width, maxBodyLines, focused, false)
}

// renderHostPane renders one host as a themed tile. extended is true only for the
// single-host view, which adds the swap/load/disk-I/O/fan panels.
func (m Model) renderHostPane(host internal.SSHHost, width, maxBodyLines int, focused, extended bool) string {
	theme := m.themeForHost(host.Name)
	base := theme.Border
	if focused {
		base = theme.FocusColor
	}

	sysInfo := m.sysInfos[host.Name]
	var body string
	if sysInfo == nil {
		body = fmt.Sprintf("%s\n%s", mutedStyle.Render("awaiting telemetry"), renderSignalBar(width-4, m.animationFrame+len(host.Name)))
	} else {
		history := m.metricHistories[host.Name]
		body = m.renderHostThemed(host.Name, func() string {
			grid := renderMetricsGrid(sysInfo, history, width-2, extended)
			headline := renderHeadline(sysInfo, width-2)
			if headline == "" {
				return grid
			}
			// The headline costs four rows. Spend them when they fit, or when the
			// grid was already going to be truncated at this height — in that case
			// the rows were being lost anyway, and a readable headline is worth
			// more than four more rows of a panel that is cut off regardless.
			withHeadline := headline + "\n\n" + grid
			if countRenderedLines(withHeadline) <= maxBodyLines || countRenderedLines(grid) > maxBodyLines {
				return withHeadline
			}
			return grid
		})
		// The metric grid uses capped card widths, so on a wide pane it can be
		// narrower than the available space. Center the block inside the pane so
		// the slack is balanced left/right instead of pooling on one side.
		body = centerBlock(body, width-2)
	}

	lines := fitLinesToPaneGradient(body, maxBodyLines, width-2, base)
	return m.renderThemedHostPanel(host, focused, strings.Join(lines, "\n"), width)
}

// centerBlock indents every non-empty line by the same amount so a content block
// narrower than width sits centered, keeping the columns' relative alignment.
func centerBlock(s string, width int) string {
	lines := strings.Split(s, "\n")
	maxW := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxW {
			maxW = w
		}
	}
	pad := (width - maxW) / 2
	if pad <= 0 {
		return s
	}
	prefix := strings.Repeat(" ", pad)
	for i, line := range lines {
		if lipgloss.Width(line) > 0 {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// fitLinesToPane top-aligns content and pads the remaining rows with blanks.
func fitLinesToPane(body string, maxBodyLines int) []string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) >= maxBodyLines {
		return lines[:maxBodyLines]
	}
	for len(lines) < maxBodyLines {
		lines = append(lines, " ")
	}
	return lines
}

// fitLinesToPaneGradient top-aligns content and fills the empty space beneath it
// with a vertical gradient that fades from the active border color down to black,
// so the slack blends into the themed border instead of being flat black.
func fitLinesToPaneGradient(body string, maxBodyLines, width int, base lipgloss.Color) []string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) >= maxBodyLines {
		return lines[:maxBodyLines]
	}
	gap := maxBodyLines - len(lines)
	return append(lines, gradientFadeToBlack(gap, width, base)...)
}

// gradientFadeToBlack returns n full-width rows whose background fades from base
// (top row) to black (bottom row).
func gradientFadeToBlack(n, width int, base lipgloss.Color) []string {
	if n <= 0 {
		return nil
	}
	width = max(1, width)
	// Keep the fill very dark: dim the border color heavily so even the top row
	// is a deep shadow of the theme color rather than the full-bright border.
	const dim = 0.18
	ramp := []colorStop{
		{pos: 0.0, red: hexRed(string(base)) * dim, green: hexGreen(string(base)) * dim, blue: hexBlue(string(base)) * dim},
		{pos: 1.0, red: 0, green: 0, blue: 0},
	}
	bar := strings.Repeat(" ", width)
	denom := math.Max(1, float64(n-1))
	rows := make([]string, n)
	for i := 0; i < n; i++ {
		color := lerpColor(float64(i)/denom, ramp)
		rows[i] = lipgloss.NewStyle().Background(color).Render(bar)
	}
	return rows
}
