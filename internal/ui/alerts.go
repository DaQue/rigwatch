package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

// Severity ranks how far a metric has crossed its configured thresholds.
type Severity int

const (
	SevOK Severity = iota
	SevWarn
	SevCrit
)

// activeThresholds is the package-level threshold set used by the (non-method)
// render helpers, mirroring how the theme system uses package-level styles. The
// Model refreshes it from its Settings at the top of View() so runtime changes
// take effect. It defaults to the built-in limits so render-only tests and the
// pre-settings code path behave exactly as before.
var activeThresholds = DefaultThresholds()

// severityFor classifies value against a warn/crit pair. A pair left at 0/0 is
// treated as disabled and never alerts.
func severityFor(value float64, t Threshold) Severity {
	if t.Crit > 0 && value >= t.Crit {
		return SevCrit
	}
	if t.Warn > 0 && value >= t.Warn {
		return SevWarn
	}
	return SevOK
}

// severityStyle overrides base only when there is an active alert, so OK readings
// keep each panel's existing look and only warn/crit recolor.
func severityStyle(sev Severity, base lipgloss.Style) lipgloss.Style {
	switch sev {
	case SevWarn:
		return warningStyle
	case SevCrit:
		return dangerStyle
	default:
		return base
	}
}

// severityGlyph returns a colored status dot for the given severity, suitable for
// list rows and headers (rendered outside a panel title).
func severityGlyph(sev Severity) string {
	switch sev {
	case SevWarn:
		return warningStyle.Render("▲")
	case SevCrit:
		return dangerStyle.Render("■")
	default:
		return successStyle.Render("●")
	}
}

// severityMarker returns a leading marker for a panel title (uncolored — the
// panel title style carries the color). Empty for OK so healthy tiles are clean.
func severityMarker(sev Severity) string {
	switch sev {
	case SevWarn:
		return "▲ "
	case SevCrit:
		return "■ "
	default:
		return ""
	}
}

// Alert is a single metric reading that crossed a threshold.
type Alert struct {
	Metric string
	Value  float64
	Unit   string
	Sev    Severity
}

// alertOnset records when a metric entered its current alert state. Alerts are
// otherwise stateless — recomputed from scratch on every render — so recency has
// to be remembered separately. The seq is a per-host poll counter rather than a
// wall clock, so every metric that trips in the same sample compares equal and
// ties break on an explicit rule instead of on clock jitter.
type alertOnset struct {
	seq int64
	sev Severity
}

// hostAlerts evaluates every monitored metric on info against t and returns the
// active warn/crit items, worst-first. A nil info yields no alerts.
func hostAlerts(info *internal.SystemInfo, t Thresholds) []Alert {
	if info == nil {
		return nil
	}
	var alerts []Alert
	add := func(metric string, value float64, unit string, th Threshold) {
		if sev := severityFor(value, th); sev != SevOK {
			alerts = append(alerts, Alert{Metric: metric, Value: value, Unit: unit, Sev: sev})
		}
	}

	add("CPU", info.CPU.UsagePercent, "%", t.CPUPct)
	if info.RAM.Total > 0 {
		add("RAM", info.RAM.UsagePercent, "%", t.RAMPct)
	}
	if info.Swap.Total > 0 {
		add("Swap", info.Swap.UsagePercent, "%", t.SwapPct)
	}
	for _, d := range info.Disk {
		if pct, ok := parsePercent(d.UsagePercent); ok {
			add("Disk "+d.MountPoint, pct, "%", t.DiskPct)
		}
	}
	for _, temp := range info.Temps {
		add(temp.Name+" temp", temp.Celsius, "°C", t.TempC)
	}
	for _, g := range info.GPUs {
		add("GPU "+g.Index+" temp", float64(g.Temperature), "°C", t.GPUTempC)
		add("GPU "+g.Index+" util", float64(g.Utilization), "%", t.GPUPct)
	}

	// Critical first, then warnings; stable within a severity (gather order).
	sortAlertsBySeverity(alerts)
	return alerts
}

func sortAlertsBySeverity(alerts []Alert) {
	// Simple stable insertion sort: small slices, preserves gather order within
	// a severity so the output reads predictably.
	for i := 1; i < len(alerts); i++ {
		for j := i; j > 0 && alerts[j].Sev > alerts[j-1].Sev; j-- {
			alerts[j], alerts[j-1] = alerts[j-1], alerts[j]
		}
	}
}

// worstSeverity is the highest severity among a host's active alerts.
func worstSeverity(info *internal.SystemInfo, t Thresholds) Severity {
	worst := SevOK
	for _, a := range hostAlerts(info, t) {
		if a.Sev > worst {
			worst = a.Sev
		}
	}
	return worst
}

func parsePercent(s string) (float64, bool) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// renderAlertsSummaryLine is the one-line health rollup shown under the overview
// header: a count of hosts in crit / warn, naming the worst offenders.
func (m Model) renderAlertsSummaryLine() string {
	var crit, warn []string
	for _, h := range m.selectedHosts {
		switch m.worstHostSeverity(h.Name) {
		case SevCrit:
			crit = append(crit, h.Name)
		case SevWarn:
			warn = append(warn, h.Name)
		}
	}
	if len(crit) == 0 && len(warn) == 0 {
		return successStyle.Render("● all hosts nominal")
	}
	var parts []string
	if len(crit) > 0 {
		parts = append(parts, dangerStyle.Render(fmt.Sprintf("■ %d critical: %s", len(crit), strings.Join(crit, ", "))))
	}
	if len(warn) > 0 {
		parts = append(parts, warningStyle.Render(fmt.Sprintf("▲ %d warning: %s", len(warn), strings.Join(warn, ", "))))
	}
	return strings.Join(parts, mutedStyle.Render("  •  "))
}

// renderAlertsSection renders the ALERTS panel for the single-host extended view,
// reading the package-level activeThresholds.
func renderAlertsSection(info *internal.SystemInfo, width int) string {
	alerts := hostAlerts(info, activeThresholds)
	if len(alerts) == 0 {
		return renderPanel("ALERTS", successStyle.Render("● all nominal"), width)
	}

	var b strings.Builder
	limit := min(len(alerts), 6)
	for i := 0; i < limit; i++ {
		a := alerts[i]
		if i != 0 {
			b.WriteString("\n")
		}
		style := severityStyle(a.Sev, panelTextStyle)
		b.WriteString(severityGlyph(a.Sev))
		b.WriteString(" ")
		b.WriteString(style.Render(fmt.Sprintf("%-18s %.0f%s", truncateVisible(a.Metric, 18), a.Value, a.Unit)))
	}
	if len(alerts) > limit {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("+%d more", len(alerts)-limit)))
	}
	// Alerts are sorted worst-first, so the head carries the panel's severity.
	return renderPanelSev("ALERTS", b.String(), width, alerts[0].Sev)
}
