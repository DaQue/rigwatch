package ui

import (
	"strings"
	"testing"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// containsBraille reports whether s holds at least one non-blank braille glyph.
func containsBraille(s string) bool {
	for _, r := range s {
		if r > 0x2800 && r <= 0x28FF {
			return true
		}
	}
	return false
}

// tipColor is the escape sequence of the last filled glyph in a bar — the cell
// whose color encodes the current reading.
func tipColor(bar, glyph string) string {
	idx := strings.LastIndex(bar, glyph)
	if idx < 0 {
		return ""
	}
	prefix := bar[:idx]
	start := strings.LastIndex(prefix, "\x1b[")
	if start < 0 {
		return ""
	}
	return prefix[start:]
}

func TestBarColorTracksValueNotBarLength(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	// Same threshold, same width, different readings: the leading edge must be a
	// different color, because color encodes the value rather than the position
	// of the last filled cell.
	cpu := Threshold{Warn: 85, Crit: 95}
	cool := renderNeonProgressBarSev(20, 40, cpu)
	hot := renderNeonProgressBarSev(98, 40, cpu)

	if tipColor(cool, "█") == tipColor(hot, "█") {
		t.Fatalf("20%% and 98%% bars share a tip color; bar color is not value-driven")
	}
}

func TestBarBelowWarnNeverReachesRedEndOfRamp(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	// A long-but-nominal bar (60% against warn 85) must stay under the ramp's
	// warn band; previously it went red simply because it was long.
	if pos := heatRampPos(60, Threshold{Warn: 85, Crit: 95}); pos >= warnRampPos {
		t.Fatalf("60%% against warn 85 mapped to ramp %.3f, want < %.3f", pos, warnRampPos)
	}
	// At and above crit the bar must be in the red band.
	if pos := heatRampPos(96, Threshold{Warn: 85, Crit: 95}); pos < critRampPos {
		t.Fatalf("96%% against crit 95 mapped to ramp %.3f, want >= %.3f", pos, critRampPos)
	}
}

func TestHeatRampPosFallsBackToRawValueWhenThresholdDisabled(t *testing.T) {
	if pos := heatRampPos(42, Threshold{}); pos < 0.41 || pos > 0.43 {
		t.Fatalf("disabled threshold mapped 42%% to %.3f, want ~0.42", pos)
	}
}

func TestPanelBorderCarriesSeverity(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	ok := renderPanelSev("CPU", "body", 40, SevOK)
	warn := renderPanelSev("CPU", "body", 40, SevWarn)
	crit := renderPanelSev("CPU", "body", 40, SevCrit)

	if ok == warn || warn == crit || ok == crit {
		t.Fatal("panels at different severities rendered identically")
	}
	// The severity marker makes the state readable without color too.
	if !strings.Contains(warn, "▲") {
		t.Fatalf("warn panel missing severity marker:\n%s", warn)
	}
	if !strings.Contains(crit, "■") {
		t.Fatalf("crit panel missing severity marker:\n%s", crit)
	}
	if strings.Contains(ok, "▲") || strings.Contains(ok, "■") {
		t.Fatalf("nominal panel should carry no marker:\n%s", ok)
	}
}

func TestPanelSeverityPreservesWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	for _, sev := range []Severity{SevOK, SevWarn, SevCrit} {
		got := renderPanelSev("CPU", "Usage: 12%", 44, sev)
		for i, line := range strings.Split(got, "\n") {
			if width := lipgloss.Width(line); width != 44 {
				t.Fatalf("sev %d line %d width = %d, want 44", sev, i, width)
			}
		}
	}
}

func TestCriticalPanelPulsesAcrossFrames(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(termenv.Ascii)
		activeFrame = 0
	})

	activeFrame = 0
	first := renderPanelSev("CPU", "body", 40, SevCrit)
	activeFrame = 4
	second := renderPanelSev("CPU", "body", 40, SevCrit)

	if first == second {
		t.Fatal("critical panel did not change between animation frames")
	}
	// A nominal panel must stay static — the pulse is a signal, not decoration.
	activeFrame = 0
	calm := renderPanelSev("CPU", "body", 40, SevOK)
	activeFrame = 4
	if calm != renderPanelSev("CPU", "body", 40, SevOK) {
		t.Fatal("nominal panel should not animate")
	}
}

func TestPanelFillSurvivesBodyColorResets(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	fill := panelFillSGR()
	if fill == "" {
		t.Fatal("expected a background fill sequence under true color")
	}
	// A body line carrying its own color ends in a reset; the fill must be
	// re-armed after it or the card background would stop partway along the row.
	body := "a" + "\x1b[0m" + "b"
	got := applyPanelFill(body, fill)
	if strings.Count(got, fill) != 2 {
		t.Fatalf("background not re-armed after inner reset: %q", got)
	}
}

func TestPanelFillSkippedWithoutTrueColor(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	if fill := panelFillSGR(); fill != "" {
		t.Fatalf("expected no background fill on a 256-color profile, got %q", fill)
	}
}

func TestBrailleTrendDoublesHorizontalResolution(t *testing.T) {
	// Twelve alternating samples in six cells: braille must show the alternation,
	// which a one-sample-per-cell renderer could not represent.
	values := []float64{0, 100, 0, 100, 0, 100, 0, 100, 0, 100, 0, 100}
	got := stripANSI(renderSparklineRows(values, 6, 1, Threshold{}))
	if lipgloss.Width(got) != 6 {
		t.Fatalf("width = %d, want 6 in %q", lipgloss.Width(got), got)
	}
	for _, r := range got {
		// Left column empty, right column full = dots 4,5,6,8 => 0x28B8.
		if r != 0x28B8 {
			t.Fatalf("expected alternating half-filled cells, got %q (%U)", got, r)
		}
	}
}

func TestBrailleTrendTwoRowsKeepEightLevels(t *testing.T) {
	// One reading per level (levels sit at multiples of 100/8) must render as a
	// distinct glyph pair, matching the vertical resolution of the old block
	// glyphs while keeping braille's doubled horizontal resolution.
	seen := make(map[string]bool)
	for _, v := range []float64{12.5, 25, 37.5, 50, 62.5, 75, 87.5, 100} {
		seen[stripANSI(renderSparklineRows([]float64{v, v}, 1, 2, Threshold{}))] = true
	}
	if len(seen) != 8 {
		t.Fatalf("two-row braille resolved %d distinct levels, want 8", len(seen))
	}
}

func TestLabeledTrendIndentsContinuationRows(t *testing.T) {
	got := stripANSI(renderLabeledTrend("TREND ", []float64{10, 90}, 8, 2, Threshold{}))
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 rows, got %d in %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "TREND ") {
		t.Fatalf("first row missing label: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], strings.Repeat(" ", len("TREND "))) {
		t.Fatalf("continuation row not indented under the label: %q", lines[1])
	}
	if lipgloss.Width(lines[0]) != lipgloss.Width(lines[1]) {
		t.Fatalf("trend rows misaligned: %d vs %d", lipgloss.Width(lines[0]), lipgloss.Width(lines[1]))
	}
}

func TestDominantMetricPicksWorstSeverity(t *testing.T) {
	activeThresholds = DefaultThresholds()
	t.Cleanup(func() { activeThresholds = DefaultThresholds() })

	// RAM is critical while CPU is merely higher in raw value: severity wins.
	info := &internal.SystemInfo{
		CPU: internal.CPUInfo{UsagePercent: 99},
		RAM: internal.RAMInfo{Total: 1000, Used: 970, UsagePercent: 97},
	}
	got, ok := dominantMetric(info)
	if !ok {
		t.Fatal("expected a headline reading")
	}
	if got.sev != SevCrit {
		t.Fatalf("headline severity = %d, want SevCrit", got.sev)
	}

	// With everything nominal the largest raw value leads.
	calm := &internal.SystemInfo{
		CPU: internal.CPUInfo{UsagePercent: 12},
		RAM: internal.RAMInfo{Total: 1000, Used: 400, UsagePercent: 40},
	}
	got, _ = dominantMetric(calm)
	if got.label != "RAM USED" {
		t.Fatalf("headline = %q, want RAM USED (the higher nominal reading)", got.label)
	}
}

func TestHeadlineRendersThreeRowsOfBlockDigits(t *testing.T) {
	info := &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 87}}
	got := renderHeadline(info, 48)
	lines := strings.Split(stripANSI(got), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 headline rows, got %d in %q", len(lines), got)
	}
	if !strings.Contains(lines[1], "CPU LOAD") {
		t.Fatalf("headline caption missing: %q", lines[1])
	}
	if !strings.ContainsAny(got, "█▀▄") {
		t.Fatalf("headline missing block digits: %q", got)
	}
}

func TestHeadlineOmittedOnNarrowTile(t *testing.T) {
	info := &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 87}}
	if got := renderHeadline(info, headlineMinWidth-1); got != "" {
		t.Fatalf("expected no headline below %d columns, got %q", headlineMinWidth, got)
	}
	if got := renderHeadline(nil, 80); got != "" {
		t.Fatalf("expected no headline without telemetry, got %q", got)
	}
}

func TestHostPaneHeadlineYieldsToPanelsWhenItWouldNotFit(t *testing.T) {
	host := internal.SSHHost{Name: "rig"}
	info := &internal.SystemInfo{
		CPU: internal.CPUInfo{Usage: "44%", UsagePercent: 44},
		RAM: internal.RAMInfo{Total: 1000, Used: 400, UsagePercent: 40},
	}
	m := Model{
		selectedHosts:   []internal.SSHHost{host},
		sysInfos:        map[string]*internal.SystemInfo{"rig": info},
		metricHistories: map[string]metricHistory{},
		settings:        DefaultSettings(),
		themePrefs:      ThemePreferences{Hosts: map[string]string{}},
	}

	grid := renderMetricsGrid(info, metricHistory{}, 78, false)
	exact := countRenderedLines(grid)

	// A pane with room for the grid but not the extra four headline rows must
	// keep the panels intact.
	if got := m.renderHostPane(host, 80, exact+2, false, false); containsBigDigits(got) {
		t.Fatalf("headline displaced panels on a pane with no spare rows:\n%s", got)
	}
	// Give it the four rows and the headline appears.
	if got := m.renderHostPane(host, 80, exact+6, false, false); !containsBigDigits(got) {
		t.Fatalf("headline missing on a pane with room for it:\n%s", got)
	}
	// A pane too short for the grid at all is already truncating, so the rows are
	// better spent on the headline.
	if got := m.renderHostPane(host, 80, exact-4, false, false); !containsBigDigits(got) {
		t.Fatalf("headline missing on an already-truncated pane:\n%s", got)
	}
}

// containsBigDigits reports whether s holds a headline block digit row.
func containsBigDigits(s string) bool {
	for _, glyph := range bigDigits {
		if strings.Contains(stripANSI(s), glyph[0]) {
			return true
		}
	}
	return false
}

func TestBigDigitsAreUniformWidth(t *testing.T) {
	for digit, glyph := range bigDigits {
		for row, line := range glyph {
			if lipgloss.Width(line) != bigDigitWidth {
				t.Fatalf("digit %q row %d width = %d, want %d", digit, row, lipgloss.Width(line), bigDigitWidth)
			}
		}
	}
}
