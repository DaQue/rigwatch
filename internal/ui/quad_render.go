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
	layout := paneLayout(m.width, m.height, total, m.quadPage)
	m.quadPage = clampInt(m.quadPage, 0, layout.Pages-1)
	m.quadFocus = clampInt(m.quadFocus, 0, max(0, layout.End-layout.Start-1))

	pageHint := ""
	if layout.Pages > 1 {
		pageHint = fmt.Sprintf("  •  page %d/%d  •  n/p page", m.quadPage+1, layout.Pages)
	}
	subtitle := fmt.Sprintf("v%s  •  refreshed %s  •  interval %s%s  •  t dashboard  •  c add hosts  •  q quit",
		internal.ShortVersion(), time.Now().Format("15:04:05"), formatInterval(m.updateInterval), pageHint)
	b.WriteString(renderHeroHeader("COMMAND CENTER // GRID", subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")

	for row := 0; row < layout.Rows; row++ {
		cells := make([]string, 0, layout.Columns)
		for col := 0; col < layout.Columns; col++ {
			idx := layout.Start + row*layout.Columns + col
			if idx < layout.End {
				cells = append(cells, m.renderQuadPanelWithFocus(m.selectedHosts[idx], layout.CellWidth, layout.BodyLines, idx-layout.Start == m.quadFocus))
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
			return renderMetricsGrid(sysInfo.CPU, sysInfo.GPUs, sysInfo.RAM, sysInfo.Disk, sysInfo.Temps, sysInfo.Network, sysInfo.Processes, history, width-2)
		})
	}

	lines := fitLinesToPaneGradient(body, maxBodyLines, width-2, base)
	return m.renderThemedHostPanel(host, focused, strings.Join(lines, "\n"), width)
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
	ramp := []colorStop{
		{pos: 0.0, red: hexRed(string(base)), green: hexGreen(string(base)), blue: hexBlue(string(base))},
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
