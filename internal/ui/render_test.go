package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func uniqueForegroundColorsForGlyph(rendered string, glyph string) int {
	re := regexp.MustCompile(`\x1b\[38;2;[0-9;]+m` + regexp.QuoteMeta(glyph))
	matches := re.FindAllString(rendered, -1)
	seen := make(map[string]bool)
	for _, match := range matches {
		seen[strings.TrimSuffix(match, glyph)] = true
	}
	return len(seen)
}

func TestFormatInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		want     string
	}{
		{name: "sub second", interval: 250 * time.Millisecond, want: "0.25s"},
		{name: "single digit seconds", interval: 1500 * time.Millisecond, want: "1.5s"},
		{name: "double digit seconds", interval: 10 * time.Second, want: "10s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatInterval(tt.interval); got != tt.want {
				t.Fatalf("formatInterval(%v) = %q, want %q", tt.interval, got, tt.want)
			}
		})
	}
}

func TestRenderGradientTextPreservesVisibleWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	got := renderGradientText("Rigwatch", coolRamp)
	if width := lipgloss.Width(got); width != len("Rigwatch") {
		t.Fatalf("visible width = %d, want %d in %q", width, len("Rigwatch"), got)
	}
	if got == "Rigwatch" {
		t.Fatalf("gradient text was not styled")
	}
}

func TestRenderGradientRuleRespectsWidth(t *testing.T) {
	got := renderGradientRule(17, coolRamp)
	if width := lipgloss.Width(got); width != 17 {
		t.Fatalf("visible width = %d, want 17 in %q", width, got)
	}
}

func TestRenderNeonProgressBarUsesHeatRampAndWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	got := renderNeonProgressBar(82, 12)
	if width := lipgloss.Width(got); width != 12 {
		t.Fatalf("visible width = %d, want 12 in %q", width, got)
	}
	if filled := strings.Count(got, "█"); filled == 0 {
		t.Fatalf("expected filled neon segments in %q", got)
	}
	if colors := uniqueForegroundColorsForGlyph(got, "█"); colors < 2 {
		t.Fatalf("expected filled progress bar gradient with multiple foreground colors, got %d in %q", colors, got)
	}
}

func TestRenderHalfHeightGradientBarUsesHalfHeightGlyphs(t *testing.T) {
	got := renderHalfHeightGradientBar(75, 10)
	if width := lipgloss.Width(got); width != 10 {
		t.Fatalf("visible width = %d, want 10 in %q", width, got)
	}
	if !strings.Contains(got, "▄") {
		t.Fatalf("expected half-height filled glyph in %q", got)
	}
	if strings.Contains(got, "█") {
		t.Fatalf("half-height core graph should not use full blocks: %q", got)
	}
}

func TestRenderThinLineGraphUsesLineGlyphs(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	got := renderThinLineGraph(64, 14)
	if width := lipgloss.Width(got); width != 14 {
		t.Fatalf("visible width = %d, want 14 in %q", width, got)
	}
	if !strings.Contains(got, "━") {
		t.Fatalf("expected heavy thin-line filled glyph in %q", got)
	}
	if strings.ContainsAny(got, "█▄") {
		t.Fatalf("thin graph should not use block glyphs: %q", got)
	}
	if colors := uniqueForegroundColorsForGlyph(got, "━"); colors < 2 {
		t.Fatalf("expected filled disk thin-line graph gradient with multiple foreground colors, got %d in %q", colors, got)
	}
}

func TestRenderSparklineUsesTrendGlyphsAndWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	got := renderSparkline([]float64{0, 25, 50, 75, 100}, 8)
	if width := lipgloss.Width(got); width != 8 {
		t.Fatalf("visible width = %d, want 8 in %q", width, got)
	}
	if !containsBraille(got) {
		t.Fatalf("expected braille sparkline glyphs in %q", got)
	}
	if colors := strings.Count(got, "\x1b[38;2;"); colors != 1 {
		t.Fatalf("expected one-color history sparkline, got %d foreground colors in %q", colors, got)
	}
}

func TestRenderSignalBarRespectsWidth(t *testing.T) {
	got := renderSignalBar(24, 9)
	if width := lipgloss.Width(got); width != 24 {
		t.Fatalf("visible width = %d, want 24 in %q", width, got)
	}
}

func TestRenderHeroHeaderUsesLargeTerminalWidth(t *testing.T) {
	got := renderHeroHeader("RIGWATCH", "status", 220, 0)
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected title, subtitle, and signal line, got %q", got)
	}
	if width := lipgloss.Width(lines[2]); width != 218 {
		t.Fatalf("signal width = %d, want 218 for a 220-column terminal", width)
	}
}

func TestRenderPanelIncludesDoubleBorderWhenWideEnough(t *testing.T) {
	got := renderPanel("CPU", "Usage: 12%", 34)
	if !strings.Contains(got, "CPU") || !strings.Contains(got, "Usage: 12%") {
		t.Fatalf("panel missing title/body: %q", got)
	}
	if width := lipgloss.Width(strings.Split(got, "\n")[0]); width != 34 {
		t.Fatalf("panel first-line width = %d, want 34 in %q", width, got)
	}
}

func TestRenderPanelUsesAssignedWidePaneWidth(t *testing.T) {
	got := renderPanel("HOST", "body", 180)
	if width := lipgloss.Width(strings.Split(got, "\n")[0]); width != 180 {
		t.Fatalf("panel width = %d, want assigned pane width 180", width)
	}
}

func TestRenderCPUSectionIncludesAggregateAndCoreMiniGraphs(t *testing.T) {
	cpu := internal.CPUInfo{
		Model:        "Ryzen Test CPU",
		Count:        "4",
		Usage:        "37.5%",
		UsagePercent: 37.5,
		Cores: []internal.CPUCoreInfo{
			{Index: 0, UsagePercent: 10},
			{Index: 1, UsagePercent: 20},
			{Index: 2, UsagePercent: 80},
			{Index: 3, UsagePercent: 95},
		},
	}

	got := renderCPUSectionWithHistory(cpu, nil, 96, false)
	for _, want := range []string{"CPU LOAD", "37.5%", "00", "03", "▄"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered CPU section missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "CORE") {
		t.Fatalf("core mini graphs should use dense numeric labels, got %q", got)
	}
}

func TestRenderCPUSectionAllocatesAllDeclaredCoreSlots(t *testing.T) {
	cpu := internal.CPUInfo{
		Count:        "4",
		Usage:        "25.0%",
		UsagePercent: 25,
		Cores: []internal.CPUCoreInfo{
			{Index: 0, UsagePercent: 10},
			{Index: 1, UsagePercent: 20},
			{Index: 2, UsagePercent: 0},
			{Index: 3, UsagePercent: 0},
		},
	}

	got := renderCPUSectionWithHistory(cpu, nil, 96, false)
	for _, want := range []string{"00", "01", "02", "03"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing stable core slot %q: %q", want, got)
		}
	}
}

func TestRenderCoreMiniGraphsUsesDenseMultiColumnLayout(t *testing.T) {
	cores := []internal.CPUCoreInfo{
		{Index: 0, UsagePercent: 10},
		{Index: 1, UsagePercent: 20},
		{Index: 2, UsagePercent: 30},
		{Index: 3, UsagePercent: 40},
		{Index: 4, UsagePercent: 50},
		{Index: 5, UsagePercent: 60},
		{Index: 6, UsagePercent: 70},
		{Index: 7, UsagePercent: 80},
	}

	got := renderCoreMiniGraphs(cores, 96)
	lines := strings.Split(got, "\n")
	if len(lines) > 2 {
		t.Fatalf("expected 8 cores to fit in at most 2 rows, got %d rows: %q", len(lines), got)
	}
	for _, want := range []string{"00", "01", "02", "03", "04", "05", "06", "07", "▄"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dense core graph missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "CORE") {
		t.Fatalf("dense core graph should not waste space on CORE labels: %q", got)
	}
}

func TestRenderDiskSectionUsesThinLineGraphs(t *testing.T) {
	disks := []internal.DiskInfo{{MountPoint: "/", Used: "42G", Size: "100G", UsagePercent: "42%"}}
	got := renderDiskSection(disks, 80)
	for _, want := range []string{"DISK ARRAY", "/", "42G/100G", "━"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disk section missing %q: %q", want, got)
		}
	}
	if strings.ContainsAny(got, "█▄") {
		t.Fatalf("disk section should use thin-line graphs, got %q", got)
	}
}

func TestRenderDiskSectionAlignsGraphColumns(t *testing.T) {
	disks := []internal.DiskInfo{
		{MountPoint: "/", Used: "42G", Size: "100G", UsagePercent: "42%"},
		{MountPoint: "/very-long-mount", Used: "900G", Size: "1000G", UsagePercent: "90%"},
	}
	got := renderDiskSection(disks, 80)

	var graphStart = -1
	var graphWidth = -1
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "G/") {
			continue
		}
		start := strings.IndexAny(line, "━─")
		if start < 0 {
			t.Fatalf("disk row missing graph: %q\n%s", line, got)
		}
		visibleStart := lipgloss.Width(line[:start])
		width := lipgloss.Width(line[start : len(line)-1])
		if graphStart == -1 {
			graphStart = visibleStart
			graphWidth = width
			continue
		}
		if visibleStart != graphStart || width != graphWidth {
			t.Fatalf("disk graphs should align and have equal width, got start=%d/%d width=%d/%d\n%s", graphStart, visibleStart, graphWidth, width, got)
		}
	}
}

func TestRenderNetworkSectionShowsRxTxRates(t *testing.T) {
	network := []internal.NetworkInfo{{Name: "eth0", RXBps: 2 * 1024 * 1024, TXBps: 512 * 1024}}
	got := renderNetworkSection(network, nil, 80)
	for _, want := range []string{"NETWORK I/O", "eth0", "2.0 MB/s", "512.0 KB/s", "IFACE", "RX", "TX"} {
		if !strings.Contains(got, want) {
			t.Fatalf("network section missing %q: %q", want, got)
		}
	}
}

func TestRenderTemperatureSectionShowsSensors(t *testing.T) {
	temps := []internal.TemperatureInfo{{Name: "x86_pkg_temp", Celsius: 55.4}, {Name: "nvme", Celsius: 42.1}}
	got := renderTemperatureSection(temps, nil, nil, 80)
	for _, want := range []string{"TEMPERATURES", "x86_pkg_temp", "55.4°C", "nvme", "42.1°C"} {
		if !strings.Contains(got, want) {
			t.Fatalf("temperature section missing %q: %q", want, got)
		}
	}
}

func TestRenderProcessSectionShowsTopProcesses(t *testing.T) {
	processes := []internal.ProcessInfo{{PID: 1234, Command: "postgres", CPUPercent: 42.5, MemPercent: 12.3}}
	got := renderProcessSection(processes, 80)
	for _, want := range []string{"TOP PROCESSES", "1234", "postgres", "42.5", "12.3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("process section missing %q: %q", want, got)
		}
	}
}

func TestRenderQuadPanelMirrorsSingleDashboardSections(t *testing.T) {
	host := internal.SSHHost{Name: "box"}
	info := sampleLargeSystemInfo()
	m := Model{
		sysInfos:        map[string]*internal.SystemInfo{"box": info},
		metricHistories: map[string]metricHistory{},
	}

	got := m.renderQuadPanel(host, 108, 30)
	for _, want := range []string{"CPU LOAD", "GPU", "RAM MATRIX", "DISK ARRAY", "NETWORK I/O", "TOP PROCESSES"} {
		if !strings.Contains(got, want) {
			t.Fatalf("quad pane should mirror single dashboard section %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "CPU   50.0%") {
		t.Fatalf("quad pane should not use the old compact summary rows:\n%s", got)
	}
}

func sampleLargeSystemInfo() *internal.SystemInfo {
	processes := make([]internal.ProcessInfo, 0, 8)
	for i := 0; i < 8; i++ {
		processes = append(processes, internal.ProcessInfo{PID: 1000 + i, Command: "proc-0" + string(rune('0'+i)), CPUPercent: float64(80 - i), MemPercent: float64(i)})
	}
	return &internal.SystemInfo{
		CPU: internal.CPUInfo{Model: "CPU", Count: "16", Usage: "50.0%", UsagePercent: 50},
		RAM: internal.RAMInfo{Total: 32000, Used: 16000, UsagePercent: 50},
		GPUs: []internal.GPUInfo{
			{Index: "0", Name: "RTX Test", VRAMTotal: 24000, VRAMUsed: 12000, Utilization: 65, PowerDraw: 250, PowerLimit: 350, Temperature: 70},
		},
		Disk: []internal.DiskInfo{
			{MountPoint: "/", Used: "42G", Size: "100G", UsagePercent: "42%"},
			{MountPoint: "/home", Used: "80G", Size: "200G", UsagePercent: "40%"},
			{MountPoint: "/data1", Used: "300G", Size: "1T", UsagePercent: "30%"},
			{MountPoint: "/data2", Used: "400G", Size: "1T", UsagePercent: "40%"},
		},
		Network: []internal.NetworkInfo{
			{Name: "eth0", RXBps: 1024 * 1024, TXBps: 512 * 1024},
			{Name: "eth1", RXBps: 2 * 1024 * 1024, TXBps: 1024 * 1024},
			{Name: "eth2", RXBps: 3 * 1024 * 1024, TXBps: 2 * 1024 * 1024},
		},
		Processes: processes,
	}
}

func TestRenderMetricsGridStretchesProcessesToNetworkBottom(t *testing.T) {
	info := &internal.SystemInfo{
		CPU: internal.CPUInfo{Model: "CPU", Count: "16", Usage: "50.0%", UsagePercent: 50, Cores: []internal.CPUCoreInfo{
			{Index: 0, UsagePercent: 25}, {Index: 1, UsagePercent: 75}, {Index: 2, UsagePercent: 10}, {Index: 3, UsagePercent: 20},
			{Index: 4, UsagePercent: 30}, {Index: 5, UsagePercent: 40}, {Index: 6, UsagePercent: 50}, {Index: 7, UsagePercent: 60},
			{Index: 8, UsagePercent: 70}, {Index: 9, UsagePercent: 80}, {Index: 10, UsagePercent: 90}, {Index: 11, UsagePercent: 95},
		}},
		RAM: internal.RAMInfo{Total: 16000, Used: 8000, UsagePercent: 50},
		Disk: []internal.DiskInfo{
			{MountPoint: "/", Used: "42G", Size: "100G", UsagePercent: "42%"},
			{MountPoint: "/home", Used: "70G", Size: "200G", UsagePercent: "35%"},
			{MountPoint: "/var", Used: "50G", Size: "100G", UsagePercent: "50%"},
			{MountPoint: "/srv", Used: "20G", Size: "80G", UsagePercent: "25%"},
		},
		Temps: []internal.TemperatureInfo{{Name: "x86_pkg_temp", Celsius: 55.4}},
		Network: []internal.NetworkInfo{
			{Name: "eth0", RXBps: 2 * 1024 * 1024, TXBps: 512 * 1024},
			{Name: "wlan0", RXBps: 1024 * 1024, TXBps: 256 * 1024},
			{Name: "enp7s0", RXBps: 512 * 1024, TXBps: 128 * 1024},
			{Name: "tailscale0", RXBps: 128 * 1024, TXBps: 64 * 1024},
		},
		Processes: []internal.ProcessInfo{{PID: 1234, Command: "postgres", CPUPercent: 42.5, MemPercent: 12.3}},
		GPUs:      []internal.GPUInfo{{Index: "0", Name: "RTX Test", VRAMTotal: 24000, VRAMUsed: 12000, Utilization: 65, PowerDraw: 250, PowerLimit: 350, Temperature: 70}},
	}

	got := renderDashboardWithHistory("test", info, metricHistory{}, time.Second, time.Unix(0, 0), 120, 40, false, 0)
	lines := strings.Split(got, "\n")
	networkTop := -1
	for i, line := range lines {
		if strings.Contains(line, "NETWORK I/O") {
			networkTop = i
			break
		}
	}
	if networkTop == -1 {
		t.Fatalf("missing NETWORK I/O panel:\n%s", got)
	}
	for i := networkTop + 1; i < len(lines); i++ {
		if strings.Contains(lines[i], "╰") {
			if strings.Count(lines[i], "╰") < 2 {
				t.Fatalf("expected TOP PROCESSES bottom border to align with NETWORK I/O bottom border on line %d:\n%s", i, got)
			}
			return
		}
	}
	t.Fatalf("missing NETWORK I/O bottom border:\n%s", got)
}

func TestRenderDashboardWideLayoutPlacesRamUnderGPUAndDiskUnderCPU(t *testing.T) {
	info := &internal.SystemInfo{
		CPU:  internal.CPUInfo{Model: "CPU", Count: "2", Usage: "50.0%", UsagePercent: 50, Cores: []internal.CPUCoreInfo{{Index: 0, UsagePercent: 25}}},
		RAM:  internal.RAMInfo{Total: 16000, Used: 8000, UsagePercent: 50},
		Disk: []internal.DiskInfo{{MountPoint: "/", Used: "42G", Size: "100G", UsagePercent: "42%"}},
		GPUs: []internal.GPUInfo{{Index: "0", Name: "RTX Test", VRAMTotal: 24000, VRAMUsed: 12000, Utilization: 65, PowerDraw: 250, PowerLimit: 350, Temperature: 70}},
	}

	got := renderDashboardWithHistory("test", info, metricHistory{}, time.Second, time.Unix(0, 0), 120, 40, false, 0)
	var topLine, secondRowLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "CPU LOAD") && strings.Contains(line, "GPU") {
			topLine = line
		}
		if strings.Contains(line, "DISK ARRAY") && strings.Contains(line, "RAM MATRIX") {
			secondRowLine = line
		}
	}
	if topLine == "" {
		t.Fatalf("expected CPU and GPU to share the top row:\n%s", got)
	}
	if secondRowLine == "" {
		t.Fatalf("expected DISK and RAM to share the second row:\n%s", got)
	}
	if strings.Index(secondRowLine, "DISK ARRAY") > strings.Index(secondRowLine, "RAM MATRIX") {
		t.Fatalf("expected DISK in first column and RAM in second column, got row %q", secondRowLine)
	}

	topColumns := strings.Split(topLine, "    ")
	secondRowColumns := strings.Split(secondRowLine, "    ")
	if len(topColumns) < 2 || len(secondRowColumns) < 2 {
		t.Fatalf("expected two-column rows, got top=%q lower=%q", topLine, secondRowLine)
	}
	cpuColumnWidth := lipgloss.Width(topColumns[0])
	diskColumnWidth := lipgloss.Width(secondRowColumns[0])
	if cpuColumnWidth != diskColumnWidth {
		t.Fatalf("expected disk column width to equal CPU column width, got cpu=%d disk=%d\ntop: %q\nrow: %q", cpuColumnWidth, diskColumnWidth, topLine, secondRowLine)
	}
}

func TestRenderDashboardTallLayoutStaysComposed(t *testing.T) {
	got := renderDashboardWithHistory("test", sampleLargeSystemInfo(), metricHistory{}, time.Second, time.Unix(0, 0), 140, 70, false, 0)
	if strings.Contains(got, "proc-04") {
		t.Fatalf("single-host dashboard should stay composed instead of expanding long process lists:\n%s", got)
	}
	if !strings.Contains(got, "TOP PROCESSES") {
		t.Fatalf("single-host dashboard should still include the composed process panel:\n%s", got)
	}
}

func TestRenderDashboardNeverExceedsTerminalHeight(t *testing.T) {
	// A content-heavy host on a short terminal must not run off the bottom border.
	const height = 24
	got := renderDashboardWithHistory("test", sampleLargeSystemInfo(), metricHistory{}, time.Second, time.Unix(0, 0), 90, height, false, 0)
	if lines := countRenderedLines(got); lines > height {
		t.Fatalf("dashboard rendered %d lines but terminal is only %d tall:\n%s", lines, height, got)
	}
}

func TestRenderDashboardPrioritizesCpuAndGpuBeforeRamAndDisk(t *testing.T) {
	info := &internal.SystemInfo{
		CPU:  internal.CPUInfo{Model: "CPU", Count: "2", Usage: "50.0%", UsagePercent: 50, Cores: []internal.CPUCoreInfo{{Index: 0, UsagePercent: 25}}},
		RAM:  internal.RAMInfo{Total: 16000, Used: 8000, UsagePercent: 50},
		Disk: []internal.DiskInfo{{MountPoint: "/", Used: "42G", Size: "100G", UsagePercent: "42%"}},
		GPUs: []internal.GPUInfo{{Index: "0", Name: "RTX Test", VRAMTotal: 24000, VRAMUsed: 12000, Utilization: 65, PowerDraw: 250, PowerLimit: 350, Temperature: 70}},
	}

	got := renderDashboardWithHistory("test", info, metricHistory{}, time.Second, time.Unix(0, 0), 120, 40, false, 0)
	for _, want := range []string{"CPU LOAD", "GPU", "RAM MATRIX", "DISK ARRAY", "00"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dashboard missing %q: %q", want, got)
		}
	}
	var topLine, lowerLine string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "CPU LOAD") && strings.Contains(line, "GPU") {
			topLine = line
		}
		if strings.Contains(line, "DISK ARRAY") && strings.Contains(line, "RAM MATRIX") {
			lowerLine = line
		}
	}
	if topLine == "" || lowerLine == "" || strings.Index(lowerLine, "DISK ARRAY") > strings.Index(lowerLine, "RAM MATRIX") {
		t.Fatalf("expected top row CPU/GPU and lower row DISK/RAM, got:\n%s", got)
	}
}
