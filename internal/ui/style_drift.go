package ui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// drift is the flat, airy look: no boxes, a gutter rule instead of a border,
// thin segmented gauges and a hairline header that breathes. Its panels are the
// same size as classic ones (title line + body + one spacer line), so the grid
// layout math is shared.

const (
	driftGutter = "▎"
	driftRule   = "╌"
	driftFull   = "▰"
	driftEmpty  = "▱"
	driftTick   = "┆"
)

func driftStyle() Style {
	return Style{
		Name:  styleDrift,
		Blurb: "flat editorial sections, thin gauges",
		Panel: driftPanel,
		Hero:  driftHero,
		Bar:   driftBar,
	}
}

func driftPanel(title, body string, width int, sev Severity) string {
	width = max(16, width)
	innerWidth := width - 2

	gutterStyle, titleStyle := panelBorderStyle, panelTextStyle.Bold(true)
	switch sev {
	case SevWarn:
		gutterStyle = lipgloss.NewStyle().Foreground(warningColor).Bold(true)
		titleStyle = gutterStyle
	case SevCrit:
		gutterStyle = lipgloss.NewStyle().Foreground(pulseColor(dangerColor, activeFrame)).Bold(true)
		titleStyle = gutterStyle
	}
	gutter := gutterStyle.Render(driftGutter) + " "

	titleText := severityMarker(sev) + title
	if lipgloss.Width(titleText) > innerWidth-1 {
		titleText = truncateVisible(titleText, innerWidth-1)
	}
	ruleWidth := max(0, innerWidth-lipgloss.Width(titleText)-1)

	var b strings.Builder
	b.WriteString(gutter)
	b.WriteString(titleStyle.Render(titleText))
	b.WriteString(" ")
	b.WriteString(emptyBarStyle.Render(strings.Repeat(driftRule, ruleWidth)))

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for _, line := range lines {
		if lipgloss.Width(line) > innerWidth {
			line = truncateVisible(line, innerWidth)
		}
		b.WriteString("\n")
		b.WriteString(gutter)
		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(line))))
	}
	// The spacer line keeps height equal to a classic panel and gives sections air.
	b.WriteString("\n")
	b.WriteString(strings.Repeat(" ", width))
	return b.String()
}

// driftHero is a quiet left-aligned header: the product name muted, the screen
// name bright, then a hairline that slowly swells toward the accent color.
func driftHero(title, subtitle string, width, frame int) string {
	width = max(40, width)
	var b strings.Builder
	if head, tail, ok := strings.Cut(title, " // "); ok {
		b.WriteString(mutedStyle.Render(strings.ToLower(head) + " / "))
		b.WriteString(panelTextStyle.Bold(true).Render(tail))
	} else {
		b.WriteString(panelTextStyle.Bold(true).Render(title))
	}
	if activeToast != "" {
		b.WriteString("   " + accentStyle.Render("✦ "+activeToast))
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(subtitle))
	b.WriteString("\n")

	swell := 0.5 + 0.5*math.Sin(float64(frame)*0.05)
	ruleColor := blendHex(string(panelDimColor), string(cyanColor), 0.25+0.45*swell)
	b.WriteString(lipgloss.NewStyle().Foreground(ruleColor).Render(strings.Repeat("─", width-2)))
	return b.String()
}

// driftBar is a thin segmented gauge colored by the same heat ramp as classic.
// A faint tick marks the warn threshold on the empty track, so the distance to
// trouble is visible without reading a number.
func driftBar(percent float64, width int, t Threshold, _, _ string) string {
	width = clampInt(width, 1, 120)
	percent = math.Max(0, math.Min(100, percent))
	filled := clampInt(int(math.Round(float64(width)*percent/100.0)), 0, width)
	tip := heatRampPos(percent, t)
	warnCell := -1
	if t.Warn > 0 && t.Warn < 100 {
		warnCell = clampInt(int(math.Round(float64(width)*t.Warn/100.0)), 0, width-1)
	}

	var b strings.Builder
	for i := 0; i < width; i++ {
		switch {
		case i < filled:
			frac := 1.0
			if filled > 1 {
				frac = float64(i) / float64(filled-1)
			}
			b.WriteString(lipgloss.NewStyle().Foreground(lerpColor(tip*frac, heatRamp)).Render(driftFull))
		case i == warnCell:
			b.WriteString(mutedStyle.Render(driftTick))
		default:
			b.WriteString(emptyBarStyle.Render(driftEmpty))
		}
	}
	return b.String()
}
