package ui

import (
	"fmt"
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
	lines := make([]string, 0, maxBodyLines)
	lines = append(lines, mutedStyle.Render("no host"))
	for len(lines) < maxBodyLines {
		lines = append(lines, " ")
	}
	return renderPanel("◇", strings.Join(lines, "\n"), width)
}

func (m Model) renderQuadPanel(host internal.SSHHost, width, maxBodyLines int) string {
	return m.renderQuadPanelWithFocus(host, width, maxBodyLines, false)
}

func (m Model) renderQuadPanelWithFocus(host internal.SSHHost, width, maxBodyLines int, focused bool) string {
	sysInfo := m.sysInfos[host.Name]
	if sysInfo == nil {
		body := fmt.Sprintf("%s\n%s", mutedStyle.Render("awaiting telemetry"), renderSignalBar(width-4, m.animationFrame+len(host.Name)))
		return m.renderThemedHostPanel(host, focused, body, width)
	}

	history := m.metricHistories[host.Name]
	body := m.renderHostThemed(host.Name, func() string {
		return renderMetricsGrid(sysInfo.CPU, sysInfo.GPUs, sysInfo.RAM, sysInfo.Disk, sysInfo.Temps, sysInfo.Network, sysInfo.Processes, history, width-2)
	})
	lines := fitLinesToPane(body, maxBodyLines)

	return m.renderThemedHostPanel(host, focused, strings.Join(lines, "\n"), width)
}

func fitLinesToPane(body string, maxBodyLines int) []string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) > maxBodyLines {
		lines = lines[:maxBodyLines]
	}
	for len(lines) < maxBodyLines {
		lines = append(lines, " ")
	}
	return lines
}
