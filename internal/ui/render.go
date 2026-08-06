package ui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderConnectingProgress() string {
	var b strings.Builder

	connectedCount := len(m.clients)
	totalCount := len(m.selectedHosts)
	subtitle := fmt.Sprintf("v%s  •  acquiring %d host signal(s)  •  %d/%d ready",
		internal.ShortVersion(), totalCount, connectedCount, totalCount)

	b.WriteString(renderHeroHeader("RIGWATCH // CONNECTING", subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")

	maxNameLen := 0
	for _, host := range m.selectedHosts {
		if len(host.Name) > maxNameLen {
			maxNameLen = len(host.Name)
		}
	}

	cardWidth := clampInt(m.width-4, 36, 82)
	for _, host := range m.selectedHosts {
		client := m.clients[host.Name]
		sysInfo := m.sysInfos[host.Name]

		statusIcon := m.spinner.View()
		statusText := mutedStyle.Render("dialing secure channel")
		if client != nil {
			if sysInfo != nil {
				statusIcon = successStyle.Render("◆")
				statusText = successStyle.Render("telemetry locked")
			} else {
				statusText = warningStyle.Render("sampling machine state")
			}
		}

		paddedName := host.Name + strings.Repeat(" ", maxNameLen-len(host.Name))
		body := fmt.Sprintf("%s  %s  %s\n%s", statusIcon, panelTextStyle.Render(paddedName), statusText, renderSignalBar(cardWidth-4, m.animationFrame+len(host.Name)))
		b.WriteString(renderPanel("HOST LINK", body, cardWidth))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("Press q to abort. Signal sweep is cosmetic; metric refresh remains on your configured interval."))
	return b.String()
}

// renderSingleHostTile renders one host as a single full-screen quad-style pane,
// so the single-host view matches the grid's tile look (themed border, mirrored
// sections, gradient fill) instead of a bare metrics grid.
func (m Model) renderSingleHostTile(host internal.SSHHost, indicator string) string {
	var b strings.Builder

	navHint := ""
	if len(m.selectedHosts) > 1 {
		navHint = "  •  n next  •  t overview  •  g grid"
	}
	lastUpdate := m.lastUpdates[host.Name]
	uptimeHint := ""
	if info := m.sysInfos[host.Name]; info != nil && info.Uptime > 0 {
		uptimeHint = "  •  up " + formatUptime(info.Uptime)
	}
	subtitle := fmt.Sprintf("v%s  •  refreshed %s%s  •  interval %s%s  •  s shell  •  c add hosts  •  v modes  •  ? help  •  q quit",
		internal.ShortVersion(), lastUpdate.Format("15:04:05"), uptimeHint, formatInterval(m.updateInterval), navHint)

	header := renderHeroHeader("RIGWATCH // "+host.Name+indicator, subtitle, m.width, m.animationFrame)
	b.WriteString(header)
	b.WriteString("\n\n")

	// Give the pane all the room left below the header (minus its own border).
	bodyLines := max(4, m.height-countRenderedLines(header)-3)
	b.WriteString(m.renderHostPane(host, m.width, bodyLines, false, true))

	out := b.String()
	if m.height > 0 {
		out = clampToHeight(out, m.height)
	}
	return out
}

func (m Model) renderOverview() string {
	var b strings.Builder

	subtitle := fmt.Sprintf("v%s  •  refreshed %s  •  interval %s  •  t per-host  •  g grid  •  c add hosts  •  v modes  •  ? help  •  q quit",
		internal.ShortVersion(), time.Now().Format("15:04:05"), formatInterval(m.updateInterval))
	b.WriteString(renderHeroHeader(fmt.Sprintf("COMMAND CENTER // %d HOSTS", len(m.selectedHosts)), subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")
	b.WriteString(m.renderAlertsSummaryLine())
	b.WriteString("\n")

	layout := paneLayout(m.width, m.height, len(m.selectedHosts), 0, quadPageSize)
	for i := 0; i < len(m.selectedHosts); i += layout.PageSize() {
		cells := make([]string, 0, layout.PageSize())
		for col := 0; col < layout.PageSize() && i+col < len(m.selectedHosts); col++ {
			cells = append(cells, m.renderSingleHostOverview(m.selectedHosts[i+col], layout.CellWidth))
		}
		b.WriteString(joinPaneRows(cells, layout.Columns))
	}

	return b.String()
}

func (m Model) renderSingleHostOverview(host internal.SSHHost, width int) string {
	return m.renderHostThemed(host.Name, func() string {
		return m.renderSingleHostOverviewThemed(host, width)
	})
}

func (m Model) renderSingleHostOverviewThemed(host internal.SSHHost, width int) string {
	sysInfo := m.sysInfos[host.Name]
	if sysInfo == nil {
		body := fmt.Sprintf("%s\n%s", mutedStyle.Render("awaiting telemetry"), renderSignalBar(width-4, m.animationFrame+len(host.Name)))
		return renderPanel("◈ "+host.Name, body, width)
	}

	var b strings.Builder
	// Lead with the worst reading rendered large, so a wall of tiles is scannable
	// without reading any of the small type.
	if headline := renderHeadline(sysInfo, width); headline != "" {
		b.WriteString(headline)
		b.WriteString("\n\n")
	}

	cpuUsage := sysInfo.CPU.Usage
	if cpuUsage == "" {
		cpuUsage = "N/A"
	}
	b.WriteString(fmt.Sprintf("CPU   %s\n", accentStyle.Render(cpuUsage)))
	b.WriteString(renderNeonProgressBarSev(sysInfo.CPU.UsagePercent, metricBarWidth(width-8), activeThresholds.CPUPct))
	b.WriteString("\n")

	if sysInfo.RAM.Total > 0 {
		b.WriteString(fmt.Sprintf("RAM   %.1f / %.1f GB  %.0f%%\n",
			float64(sysInfo.RAM.Used)/1024,
			float64(sysInfo.RAM.Total)/1024,
			sysInfo.RAM.UsagePercent))
		b.WriteString(renderNeonProgressBarSev(sysInfo.RAM.UsagePercent, metricBarWidth(width-8), activeThresholds.RAMPct))
		b.WriteString("\n")
	} else {
		b.WriteString("RAM   N/A\n")
	}

	if len(sysInfo.Disk) > 0 {
		disk := sysInfo.Disk[0]
		b.WriteString(fmt.Sprintf("DISK  %s / %s  %s\n", disk.Used, disk.Size, disk.UsagePercent))
	} else {
		b.WriteString("DISK  N/A\n")
	}

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
		b.WriteString(fmt.Sprintf("GPU   %d%% avg  VRAM %.0f%%\n", avgUtil, vramPercent))
		b.WriteString(renderNeonProgressBarSev(float64(avgUtil), metricBarWidth(width-8), activeThresholds.GPUPct))
	} else {
		b.WriteString(mutedStyle.Render("GPU   not detected"))
	}

	return renderPanel("◈ "+host.Name, b.String(), width)
}

func renderDashboardWithHistory(hostName string, info *internal.SystemInfo, history metricHistory, updateInterval time.Duration, lastUpdate time.Time, width, height int, multiHost bool, frame int) string {
	var b strings.Builder

	navHint := ""
	if multiHost {
		navHint = "  •  n next  •  t overview  •  g grid"
	}
	subtitle := fmt.Sprintf("v%s  •  refreshed %s  •  interval %s%s  •  s shell  •  c add hosts  •  v modes  •  ? help  •  q quit",
		internal.ShortVersion(), lastUpdate.Format("15:04:05"), formatInterval(updateInterval), navHint)

	header := renderHeroHeader("RIGWATCH // "+hostName, subtitle, width, frame)
	b.WriteString(header)
	b.WriteString("\n\n")

	b.WriteString(renderMetricsGrid(info, history, width, false))

	out := b.String()
	// Hard guard: never emit more rows than the terminal has, so a tall host
	// can't run off the bottom border.
	if height > 0 {
		out = clampToHeight(out, height)
	}
	return out
}

// clampToHeight trims rendered output so it never exceeds height terminal rows.
func clampToHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	return strings.Join(lines[:height], "\n")
}

// extendedProcessRows is the process-panel height (header + rows) used in the
// single-host view, where there is room for a real top-process list instead of
// the column-balanced count the compact quad layout uses.
const extendedProcessRows = 9

// renderMetricsGrid lays out the metric panels. When extended is true (single-
// host view only) it also includes the swap, load-average, disk-I/O and fan
// panels; when false the output is identical to the compact quad layout.
func renderMetricsGrid(info *internal.SystemInfo, history metricHistory, width int, extended bool) string {
	cpu, gpus, ram := info.CPU, info.GPUs, info.RAM
	disks, temps, network, processes := info.Disk, info.Temps, info.Network, info.Processes

	if width >= 156 {
		cardWidth := clampInt((width-8)/3, 46, 68)
		left := []string{
			renderCPUSectionWithHistory(cpu, history.CPU, cardWidth, extended),
			renderDiskSection(disks, cardWidth),
		}
		middle := []string{
			renderGPUSummarySectionWithHistory(gpus, history.GPU, history.VRAM, cardWidth, extended),
			renderRAMSectionWithHistory(ram, history.RAM, cardWidth, extended),
			renderTemperatureSection(temps, gpus, history.Temp, cardWidth),
		}
		right := []string{renderNetworkSection(network, history.Network, cardWidth)}
		if extended {
			left = append([]string{renderAlertsSection(info, cardWidth)}, left...)
			left = append(left, renderDiskIOSection(info.DiskIO, cardWidth))
			middle = append(middle, renderSwapSection(info.Swap, cardWidth))
			right = append(right, renderLoadSection(info.Load, cardWidth), renderFanSection(info.Fans, history.Fans, cardWidth))
		}
		if extended {
			right = append(right, renderProcessSectionWithRows(processes, cardWidth, extendedProcessRows))
		} else {
			right = append(right, renderProcessSection(processes, cardWidth))
		}
		return joinGridCells([]string{strings.Join(left, "\n"), strings.Join(middle, "\n"), strings.Join(right, "\n")}) + "\n"
	}

	if width >= 104 {
		cardWidth := clampInt((width-6)/2, 46, 68)
		leftWidth := clampInt(cardWidth-2, 38, 120)
		left := []string{
			renderCPUSectionWithHistory(cpu, history.CPU, leftWidth, extended),
			renderDiskSection(disks, leftWidth),
			renderNetworkSection(network, history.Network, leftWidth),
		}
		rightPrefixPanels := []string{
			renderGPUSummarySectionWithHistory(gpus, history.GPU, history.VRAM, cardWidth, extended),
			renderRAMSectionWithHistory(ram, history.RAM, cardWidth, extended),
			renderTemperatureSection(temps, gpus, history.Temp, cardWidth),
		}
		if extended {
			left = append([]string{renderAlertsSection(info, leftWidth)}, left...)
			left = append(left, renderDiskIOSection(info.DiskIO, leftWidth))
			rightPrefixPanels = append(rightPrefixPanels, renderSwapSection(info.Swap, cardWidth), renderLoadSection(info.Load, cardWidth), renderFanSection(info.Fans, history.Fans, cardWidth))
		}
		leftColumn := strings.Join(left, "\n")
		rightPrefix := strings.Join(rightPrefixPanels, "\n")
		processRows := max(1, countRenderedLines(leftColumn)-countRenderedLines(rightPrefix)-2)
		if extended {
			processRows = extendedProcessRows
		}
		rightColumn := rightPrefix + "\n" + renderProcessSectionWithRows(processes, cardWidth, processRows)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, "    ", rightColumn) + "\n"
	}

	cardWidth := clampInt(width-2, 42, 90)
	stack := []string{
		renderCPUSectionWithHistory(cpu, history.CPU, cardWidth, extended),
		renderGPUSummarySectionWithHistory(gpus, history.GPU, history.VRAM, cardWidth, extended),
		renderRAMSectionWithHistory(ram, history.RAM, cardWidth, extended),
		renderTemperatureSection(temps, gpus, history.Temp, cardWidth),
		renderDiskSection(disks, cardWidth),
		renderNetworkSection(network, history.Network, cardWidth),
	}
	if extended {
		stack = append([]string{renderAlertsSection(info, cardWidth)}, stack...)
		stack = append(stack,
			renderSwapSection(info.Swap, cardWidth),
			renderLoadSection(info.Load, cardWidth),
			renderDiskIOSection(info.DiskIO, cardWidth),
			renderFanSection(info.Fans, history.Fans, cardWidth))
	}
	if extended {
		stack = append(stack, renderProcessSectionWithRows(processes, cardWidth, extendedProcessRows))
	} else {
		stack = append(stack, renderProcessSection(processes, cardWidth))
	}
	return strings.Join(stack, "\n") + "\n"
}

func renderCPUSectionWithHistory(cpu internal.CPUInfo, history []float64, width int, extended bool) string {
	var b strings.Builder
	barWidth := metricBarWidth(width - 12)
	usagePercent := cpu.UsagePercent
	if usagePercent == 0 && strings.HasSuffix(cpu.Usage, "%") {
		if parsed, err := strconv.ParseFloat(strings.TrimSuffix(cpu.Usage, "%"), 64); err == nil {
			usagePercent = parsed
		}
	}

	var parts []string
	if cpu.Model != "" {
		parts = append(parts, abbreviateCPUModel(cpu.Model, clampInt(width-26, 24, 80)))
	}
	if cpu.Count != "" {
		parts = append(parts, fmt.Sprintf("%sc", cpu.Count))
	}
	cpuSev := severityFor(usagePercent, activeThresholds.CPUPct)
	parts = append(parts, severityStyle(cpuSev, accentStyle).Render(cpu.Usage))
	b.WriteString(panelTextStyle.Render(strings.Join(parts, "  •  ")))
	b.WriteString("\n")
	b.WriteString(renderNeonProgressBarSev(usagePercent, barWidth, activeThresholds.CPUPct))
	if len(history) > 1 {
		b.WriteString("\n")
		b.WriteString(renderLabeledTrend("TREND ", history, barWidth, trendRows(extended), activeThresholds.CPUPct))
	}

	if len(cpu.Cores) > 0 {
		b.WriteString("\n")
		b.WriteString(renderCoreMiniGraphs(cpu.Cores, width))
	}

	return renderPanelSev("CPU LOAD", b.String(), width, cpuSev)
}

func renderCoreMiniGraphs(cores []internal.CPUCoreInfo, width int) string {
	if len(cores) == 0 {
		return ""
	}

	innerWidth := clampInt(width-4, 28, 116)
	cellWidth := 14
	cols := clampInt(innerWidth/(cellWidth+1), 1, 8)
	barWidth := clampInt(cellWidth-4, 6, 10)

	rows := make([]string, 0, (len(cores)+cols-1)/cols)
	for i := 0; i < len(cores); i += cols {
		cells := make([]string, 0, cols)
		for j := 0; j < cols && i+j < len(cores); j++ {
			core := cores[i+j]
			cell := fmt.Sprintf("%02d %s", core.Index, renderHalfHeightGradientBarSev(core.UsagePercent, barWidth, activeThresholds.CPUPct))
			cells = append(cells, cell)
		}
		rows = append(rows, strings.Join(cells, " "))
	}

	return strings.Join(rows, "\n")
}

func renderGPUSummarySectionWithHistory(gpus []internal.GPUInfo, gpuHistory []float64, vramHistory []float64, width int, extended bool) string {
	if len(gpus) == 0 {
		return renderPanel("GPU", mutedStyle.Render("not detected"), clampInt(width, 42, 90))
	}

	var totalVRAM, usedVRAM int
	var totalUtil, maxTemp, totalPower, totalPowerLimit int
	for _, gpu := range gpus {
		totalVRAM += gpu.VRAMTotal
		usedVRAM += gpu.VRAMUsed
		totalUtil += gpu.Utilization
		totalPower += gpu.PowerDraw
		totalPowerLimit += gpu.PowerLimit
		if gpu.Temperature > maxTemp {
			maxTemp = gpu.Temperature
		}
	}

	avgUtil := float64(totalUtil) / float64(len(gpus))
	vramPercent := 0.0
	if totalVRAM > 0 {
		vramPercent = (float64(usedVRAM) / float64(totalVRAM)) * 100
	}

	barWidth := metricBarWidth(width - 8)
	name := truncateVisible(gpus[0].Name, clampInt(width-18, 16, 42))
	if len(gpus) > 1 {
		name = fmt.Sprintf("%d GPUs", len(gpus))
	}

	utilSev := severityFor(avgUtil, activeThresholds.GPUPct)
	tempSev := severityFor(float64(maxTemp), activeThresholds.GPUTempC)
	panelSev := utilSev
	if tempSev > panelSev {
		panelSev = tempSev
	}

	var b strings.Builder
	utilStr := severityStyle(utilSev, mutedStyle).Render(fmt.Sprintf("util %.0f%%", avgUtil))
	tempStr := severityStyle(tempSev, mutedStyle).Render(fmt.Sprintf("%d°C", maxTemp))
	b.WriteString(fmt.Sprintf("%s  %s  vram %.0f%%  %dW  %s\n", mutedStyle.Render(name), utilStr, vramPercent, totalPower, tempStr))
	b.WriteString(accentStyle.Render("UTIL "))
	b.WriteString(renderNeonProgressBarSev(avgUtil, barWidth, activeThresholds.GPUPct))
	b.WriteString("\n")
	b.WriteString(accentStyle.Render("VRAM "))
	b.WriteString(renderNeonProgressBar(vramPercent, barWidth))
	// The power-draw bar is shown only in the roomy single-host (extended) view;
	// the compact grid keeps the GPU panel short so columns stay aligned.
	if extended && totalPowerLimit > 0 {
		powerPct := float64(totalPower) / float64(totalPowerLimit) * 100
		b.WriteString("\n")
		b.WriteString(accentStyle.Render("PWR  "))
		b.WriteString(renderNeonProgressBar(powerPct, barWidth))
		b.WriteString(mutedStyle.Render(fmt.Sprintf(" %d/%dW", totalPower, totalPowerLimit)))
	}
	if len(gpuHistory) > 1 {
		b.WriteString("\n")
		b.WriteString(renderLabeledTrend("HIST ", gpuHistory, barWidth, trendRows(extended), activeThresholds.GPUPct))
	}
	if len(vramHistory) > 1 {
		b.WriteString("\n")
		b.WriteString(renderLabeledTrend("VRAM ", vramHistory, barWidth, trendRows(extended), Threshold{}))
	}

	return renderPanelSev("GPU", b.String(), clampInt(width, 42, 90), panelSev)
}

func renderRAMSectionWithHistory(ram internal.RAMInfo, history []float64, width int, extended bool) string {
	var b strings.Builder
	totalGB := float64(ram.Total) / 1024.0
	usedGB := float64(ram.Used) / 1024.0
	barWidth := metricBarWidth(width - 8)

	ramSev := SevOK
	if ram.Total > 0 {
		ramSev = severityFor(ram.UsagePercent, activeThresholds.RAMPct)
		line := fmt.Sprintf("%.1f GB / %.1f GB (%.1f%%)", usedGB, totalGB, ram.UsagePercent)
		b.WriteString(severityStyle(ramSev, panelTextStyle).Render(line))
		b.WriteString("\n")
		b.WriteString(renderNeonProgressBarSev(ram.UsagePercent, barWidth, activeThresholds.RAMPct))
		if len(history) > 1 {
			b.WriteString("\n")
			b.WriteString(renderLabeledTrend("TREND ", history, barWidth, trendRows(extended), activeThresholds.RAMPct))
		}
	} else {
		b.WriteString(mutedStyle.Render("RAM telemetry unavailable"))
	}

	return renderPanelSev("RAM MATRIX", b.String(), width, ramSev)
}

func renderDiskSection(disks []internal.DiskInfo, width int) string {
	var b strings.Builder
	mountWidth := 12
	sizeWidth := 13
	percentWidth := 4
	labelWidth := mountWidth + 1 + sizeWidth + 1 + percentWidth + 1
	barWidth := clampInt(width-2-labelWidth, 8, 44)

	if len(disks) == 0 {
		return renderPanel("DISK ARRAY", mutedStyle.Render("disk telemetry unavailable"), width)
	}

	panelSev := SevOK
	for i, disk := range disks {
		usageStr := strings.TrimSuffix(disk.UsagePercent, "%")
		usagePercent := 0.0
		if val, err := strconv.ParseFloat(usageStr, 64); err == nil {
			usagePercent = val
		}

		mount := truncateVisible(disk.MountPoint, mountWidth)
		size := truncateVisible(fmt.Sprintf("%s/%s", disk.Used, disk.Size), sizeWidth)
		// Pad before styling so ANSI codes don't throw off the column width.
		diskSev := severityFor(usagePercent, activeThresholds.DiskPct)
		if diskSev > panelSev {
			panelSev = diskSev
		}
		pct := severityStyle(diskSev, panelTextStyle).Render(fmt.Sprintf("%*s", percentWidth, disk.UsagePercent))
		b.WriteString(fmt.Sprintf("%-*s %-*s %s %s",
			mountWidth, mount, sizeWidth, size, pct, renderThinLineGraphSev(usagePercent, barWidth, activeThresholds.DiskPct)))
		if i != len(disks)-1 {
			b.WriteString("\n")
		}
	}

	return renderPanelSev("DISK ARRAY", b.String(), width, panelSev)
}

func renderNetworkSection(network []internal.NetworkInfo, history []float64, width int) string {
	if len(network) == 0 {
		return renderPanel("NETWORK I/O", mutedStyle.Render("network telemetry warming up"), width)
	}

	var b strings.Builder
	// Header row
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%-10s  %-11s  %-11s", "IFACE", "RX", "TX")))
	b.WriteString("\n")
	limit := min(len(network), 4)
	barWidth := clampInt(width-39, 4, 32)
	maxRate := 125.0 * 1024 * 1024 // 125 MB/s ≈ 1 Gbps fills the bar
	for i := 0; i < limit; i++ {
		iface := network[i]
		pct := ioRatePercent(float64(iface.RXBps+iface.TXBps), maxRate)
		bar := renderThinLineGraph(pct, barWidth)
		b.WriteString(fmt.Sprintf("%-10s  %-11s  %-11s %s",
			truncateVisible(iface.Name, 10), formatBytesPerSecond(iface.RXBps), formatBytesPerSecond(iface.TXBps), bar))
		if i != limit-1 {
			b.WriteString("\n")
		}
	}
	return renderPanel("NETWORK I/O", b.String(), width)
}

func renderSwapSection(swap internal.SwapInfo, width int) string {
	var b strings.Builder
	swapSev := SevOK
	if swap.Total > 0 {
		usedGB := float64(swap.Used) / 1024.0
		totalGB := float64(swap.Total) / 1024.0
		swapSev = severityFor(swap.UsagePercent, activeThresholds.SwapPct)
		line := fmt.Sprintf("%.1f GB / %.1f GB (%.1f%%)", usedGB, totalGB, swap.UsagePercent)
		b.WriteString(severityStyle(swapSev, panelTextStyle).Render(line))
		b.WriteString("\n")
		b.WriteString(renderNeonProgressBarSev(swap.UsagePercent, metricBarWidth(width-8), activeThresholds.SwapPct))
	} else {
		b.WriteString(mutedStyle.Render("no swap configured"))
	}
	return renderPanelSev("SWAP", b.String(), width, swapSev)
}

func renderLoadSection(load internal.LoadInfo, width int) string {
	var b strings.Builder
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%-7s %-7s %-7s", "1 MIN", "5 MIN", "15 MIN")))
	b.WriteString("\n")
	// Pad before styling: the ANSI codes accentStyle adds would otherwise be
	// counted by the width verb and push the following columns out of line.
	b.WriteString(fmt.Sprintf("%s %-7s %-7s",
		accentStyle.Render(fmt.Sprintf("%-7.2f", load.Load1)),
		fmt.Sprintf("%.2f", load.Load5),
		fmt.Sprintf("%.2f", load.Load15)))
	if load.Total > 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("%d running / %d total tasks", load.Running, load.Total)))
	}
	return renderPanel("LOAD AVG", b.String(), width)
}

func renderDiskIOSection(diskIO []internal.DiskIOInfo, width int) string {
	if len(diskIO) == 0 {
		return renderPanel("DISK I/O", mutedStyle.Render("disk I/O warming up"), width)
	}

	// Aggregate totals, and surface the busiest devices.
	var totalRead, totalWrite uint64
	for _, dev := range diskIO {
		totalRead += dev.ReadBps
		totalWrite += dev.WriteBps
	}

	busiest := make([]internal.DiskIOInfo, len(diskIO))
	copy(busiest, diskIO)
	sort.Slice(busiest, func(i, j int) bool {
		return busiest[i].ReadBps+busiest[i].WriteBps > busiest[j].ReadBps+busiest[j].WriteBps
	})

	var b strings.Builder
	b.WriteString(fmt.Sprintf("TOTAL  %s %s   %s %s\n",
		mutedStyle.Render("R"), accentStyle.Render(formatBytesPerSecond(totalRead)),
		mutedStyle.Render("W"), accentStyle.Render(formatBytesPerSecond(totalWrite))))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%-10s  %-11s  %-11s", "DEVICE", "READ", "WRITE")))
	barWidth := clampInt(width-39, 4, 32)
	maxRate := 2.0 * 1024 * 1024 * 1024 // 2 GB/s fills the bar (NVMe-class)
	limit := min(len(busiest), 3)
	for i := 0; i < limit; i++ {
		dev := busiest[i]
		pct := ioRatePercent(float64(dev.ReadBps+dev.WriteBps), maxRate)
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("%-10s  %-11s  %-11s %s",
			truncateVisible(dev.Device, 10), formatBytesPerSecond(dev.ReadBps), formatBytesPerSecond(dev.WriteBps), renderThinLineGraph(pct, barWidth)))
	}
	return renderPanel("DISK I/O", b.String(), width)
}

// maxFanRPM is the full-scale reference for fan trend bars, so bar height tracks
// RPM level (a steady fan reads flat, a ramping fan trends up).
const maxFanRPM = 3000.0

func renderFanSection(fans []internal.FanInfo, fanHistory map[string][]float64, width int) string {
	if len(fans) == 0 {
		return renderPanel("FANS", mutedStyle.Render("no fans detected"), width)
	}
	const nameWidth, rpmWidth = 12, 8 // rpmWidth fits "9999 RPM"
	sparkWidth := clampInt(width-4-nameWidth-1-rpmWidth-1, 5, 24)

	var b strings.Builder
	for i, fan := range fans {
		if i != 0 {
			b.WriteString("\n")
		}
		spark := emptyBarStyle.Render(strings.Repeat(brailleBaseline, sparkWidth))
		if hist := fanHistory[fan.Name]; len(hist) > 1 {
			norm := make([]float64, len(hist))
			for j, rpm := range hist {
				norm[j] = rpm / maxFanRPM * 100
			}
			spark = renderSparkline(norm, sparkWidth)
		}
		b.WriteString(fmt.Sprintf("%-*s %s %s",
			nameWidth, truncateVisible(fan.Name, nameWidth),
			accentStyle.Render(fmt.Sprintf("%4d RPM", fan.RPM)),
			spark))
	}
	return renderPanel("FANS", b.String(), width)
}

func renderTemperatureSection(temps []internal.TemperatureInfo, gpus []internal.GPUInfo, history []float64, width int) string {
	// Merge GPU temps as fallback/addition
	seen := make(map[string]bool)
	for _, t := range temps {
		seen[t.Name] = true
	}
	for _, g := range gpus {
		name := fmt.Sprintf("GPU %s", g.Index)
		if !seen[name] {
			temps = append(temps, internal.TemperatureInfo{Name: name, Celsius: float64(g.Temperature)})
		}
	}

	if len(temps) == 0 {
		return renderPanel("TEMPERATURES", mutedStyle.Render("temperature telemetry unavailable"), width)
	}

	var b strings.Builder
	// Header row
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%-12s  %s", "SENSOR", "TEMP")))
	b.WriteString("\n")
	limit := min(len(temps), 5)
	barWidth := clampInt(width-25, 4, 40)
	panelSev := SevOK
	for i := 0; i < limit; i++ {
		temp := temps[i]
		rawValue := fmt.Sprintf("%-6s", fmt.Sprintf("%.1f°C", temp.Celsius))
		// GPU sensors use the dedicated GPU temperature thresholds; everything
		// else uses the general temperature thresholds.
		tempThreshold := activeThresholds.TempC
		if strings.HasPrefix(temp.Name, "GPU") {
			tempThreshold = activeThresholds.GPUTempC
		}
		tempSev := severityFor(temp.Celsius, tempThreshold)
		if tempSev > panelSev {
			panelSev = tempSev
		}
		styledValue := severityStyle(tempSev, successStyle).Render(rawValue)
		// Celsius is already on a 0-100 scale, so the threshold pair (also in °C)
		// maps straight onto the bar's severity ramp.
		pct := math.Min(100, temp.Celsius)
		bar := renderThinLineGraphSev(pct, barWidth, tempThreshold)
		b.WriteString(fmt.Sprintf("%-12s  %s %s", truncateVisible(temp.Name, 12), styledValue, bar))
		if i != limit-1 {
			b.WriteString("\n")
		}
	}
	return renderPanelSev("TEMPERATURES", b.String(), width, panelSev)
}

func renderProcessSection(processes []internal.ProcessInfo, width int) string {
	return renderProcessSectionWithRows(processes, width, 2)
}

func renderProcessSectionWithRows(processes []internal.ProcessInfo, width int, minRows int) string {
	minRows = max(1, minRows)
	var rows []string
	if len(processes) == 0 {
		rows = append(rows, mutedStyle.Render("process telemetry unavailable"))
	} else {
		// Header row
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("%-7s %-18s %-5s %-5s", "PID", "COMMAND", "CPU%", "MEM%")))
		limit := min(len(processes), minRows-1)
		for i := 0; i < limit; i++ {
			proc := processes[i]
			rows = append(rows, fmt.Sprintf("%-7d %-18s %-5.1f %-5.1f",
				proc.PID, truncateVisible(proc.Command, 18), proc.CPUPercent, proc.MemPercent))
		}
	}
	for len(rows) < minRows {
		rows = append(rows, " ")
	}
	return renderPanel("TOP PROCESSES", strings.Join(rows, "\n"), width)
}

func countRenderedLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// formatUptime renders a duration compactly: "4d 3h", "3h 12m", or "12m".
func formatUptime(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func formatBytesPerSecond(bytes uint64) string {
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%.0f %s", value, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}
