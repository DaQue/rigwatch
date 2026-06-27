package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

const quadPageSize = 4

// quadPageBounds returns the [start, end) slice bounds for the given page of a
// 4-per-page grid, along with the total page count. page is clamped into range.
func quadPageBounds(total, page int) (start, end, pages int) {
	pages = (total + quadPageSize - 1) / quadPageSize
	if pages < 1 {
		pages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= pages {
		page = pages - 1
	}
	start = page * quadPageSize
	end = start + quadPageSize
	if end > total {
		end = total
	}
	if start > total {
		start = total
	}
	return start, end, pages
}

func (m Model) renderQuad() string {
	var b strings.Builder

	total := len(m.selectedHosts)
	start, end, pages := quadPageBounds(total, m.quadPage)

	pageHint := ""
	if pages > 1 {
		pageHint = fmt.Sprintf("  •  page %d/%d  •  n/p page", m.quadPage+1, pages)
	}
	subtitle := fmt.Sprintf("v%s  •  refreshed %s  •  interval %s%s  •  t dashboard  •  c add hosts  •  q quit",
		internal.ShortVersion(), time.Now().Format("15:04:05"), formatInterval(m.updateInterval), pageHint)
	b.WriteString(renderHeroHeader("COMMAND CENTER // GRID", subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")

	cardWidth := clampInt((m.width-6)/2, 44, 80)
	maxBodyLines := clampInt((m.height-8)/2-2, 6, 16)

	// Build exactly four cells for the current page, padding with placeholders.
	cells := make([]string, quadPageSize)
	for i := 0; i < quadPageSize; i++ {
		idx := start + i
		if idx < end {
			cells[i] = m.renderQuadPanel(m.selectedHosts[idx], cardWidth, maxBodyLines)
		} else {
			cells[i] = renderEmptyQuadCell(cardWidth, maxBodyLines)
		}
	}

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, cells[0], "  ", cells[1])
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, cells[2], "  ", cells[3])
	b.WriteString(topRow)
	b.WriteString("\n")
	b.WriteString(bottomRow)
	b.WriteString("\n")

	return b.String()
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
	sysInfo := m.sysInfos[host.Name]
	if sysInfo == nil {
		body := fmt.Sprintf("%s\n%s", mutedStyle.Render("awaiting telemetry"), renderSignalBar(width-4, m.animationFrame+len(host.Name)))
		return renderPanel("◈ "+host.Name, body, width)
	}

	history := m.metricHistories[host.Name]
	barWidth := metricBarWidth(width - 8)
	var lines []string

	// CPU
	cpuUsage := sysInfo.CPU.Usage
	if cpuUsage == "" {
		cpuUsage = "N/A"
	}
	lines = append(lines, fmt.Sprintf("CPU   %s", accentStyle.Render(cpuUsage)))
	lines = append(lines, renderNeonProgressBar(sysInfo.CPU.UsagePercent, barWidth))
	if len(history.CPU) > 1 {
		lines = append(lines, mutedStyle.Render("TREND ")+renderSparkline(history.CPU, barWidth))
	}

	// GPU
	if len(sysInfo.GPUs) > 0 {
		var totalVRAM, usedVRAM, totalUtil int
		for _, gpu := range sysInfo.GPUs {
			totalVRAM += gpu.VRAMTotal
			usedVRAM += gpu.VRAMUsed
			totalUtil += gpu.Utilization
		}
		vramPercent := 0.0
		if totalVRAM > 0 {
			vramPercent = (float64(usedVRAM) / float64(totalVRAM)) * 100
		}
		avgUtil := totalUtil / len(sysInfo.GPUs)
		lines = append(lines, fmt.Sprintf("GPU   %d%% util  VRAM %.0f%%", avgUtil, vramPercent))
		lines = append(lines, renderNeonProgressBar(float64(avgUtil), barWidth))
		if len(history.GPU) > 1 {
			lines = append(lines, mutedStyle.Render("TREND ")+renderSparkline(history.GPU, barWidth))
		}
	} else {
		lines = append(lines, mutedStyle.Render("GPU   not detected"))
	}

	// RAM
	if sysInfo.RAM.Total > 0 {
		lines = append(lines, fmt.Sprintf("RAM   %.1f / %.1f GB  %.0f%%",
			float64(sysInfo.RAM.Used)/1024, float64(sysInfo.RAM.Total)/1024, sysInfo.RAM.UsagePercent))
		lines = append(lines, renderNeonProgressBar(sysInfo.RAM.UsagePercent, barWidth))
		if len(history.RAM) > 1 {
			lines = append(lines, mutedStyle.Render("TREND ")+renderSparkline(history.RAM, barWidth))
		}
	} else {
		lines = append(lines, "RAM   N/A")
	}

	// DISK (first mount)
	if len(sysInfo.Disk) > 0 {
		disk := sysInfo.Disk[0]
		lines = append(lines, fmt.Sprintf("DISK  %s  %s/%s  %s",
			truncateVisible(disk.MountPoint, 10), disk.Used, disk.Size, disk.UsagePercent))
	}

	// NET (aggregate throughput)
	if len(sysInfo.Network) > 0 {
		var rx, tx uint64
		for _, iface := range sysInfo.Network {
			rx += iface.RXBps
			tx += iface.TXBps
		}
		lines = append(lines, fmt.Sprintf("NET   ↓ %s  ↑ %s", formatBytesPerSecond(rx), formatBytesPerSecond(tx)))
	}

	// TOP process
	if len(sysInfo.Processes) > 0 {
		p := sysInfo.Processes[0]
		lines = append(lines, fmt.Sprintf("TOP   %s  %.0f%%", truncateVisible(p.Command, 16), p.CPUPercent))
	}

	if len(lines) > maxBodyLines {
		lines = lines[:maxBodyLines]
	}

	return renderPanel("◈ "+host.Name, strings.Join(lines, "\n"), width)
}
