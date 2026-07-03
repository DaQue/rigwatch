package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type colorStop struct {
	pos   float64
	red   float64
	green float64
	blue  float64
}

var (
	coolRamp = []colorStop{
		{pos: 0.00, red: 0x7c, green: 0x3a, blue: 0xff},
		{pos: 0.34, red: 0xff, green: 0x4f, blue: 0xc3},
		{pos: 0.68, red: 0x32, green: 0xd9, blue: 0xff},
		{pos: 1.00, red: 0xf8, green: 0xfb, blue: 0xff},
	}
	heatRamp = []colorStop{
		{pos: 0.00, red: 0x27, green: 0xef, blue: 0x7f},
		{pos: 0.50, red: 0xf5, green: 0xe8, blue: 0x42},
		{pos: 1.00, red: 0xff, green: 0x3b, blue: 0x4b},
	}
)

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func lerpColor(t float64, ramp []colorStop) lipgloss.Color {
	t = clamp01(t)
	if len(ramp) == 0 {
		return lipgloss.Color("15")
	}
	if len(ramp) == 1 || t <= ramp[0].pos {
		return colorFromStop(ramp[0])
	}

	for i := 1; i < len(ramp); i++ {
		if t <= ramp[i].pos {
			prev := ramp[i-1]
			next := ramp[i]
			span := next.pos - prev.pos
			if span <= 0 {
				return colorFromStop(next)
			}
			frac := (t - prev.pos) / span
			return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
				uint8(math.Round(prev.red+(next.red-prev.red)*frac)),
				uint8(math.Round(prev.green+(next.green-prev.green)*frac)),
				uint8(math.Round(prev.blue+(next.blue-prev.blue)*frac))))
		}
	}

	return colorFromStop(ramp[len(ramp)-1])
}

func colorFromStop(stop colorStop) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", uint8(stop.red), uint8(stop.green), uint8(stop.blue)))
}

func renderGradientText(text string, ramp []colorStop) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	denom := math.Max(1, float64(len(runes)-1))

	var b strings.Builder
	for i, r := range runes {
		color := lerpColor(float64(i)/denom, ramp)
		b.WriteString(lipgloss.NewStyle().Foreground(color).Bold(true).Render(string(r)))
	}
	return b.String()
}

func renderGradientRule(width int, ramp []colorStop) string {
	if width <= 0 {
		return ""
	}
	denom := math.Max(1, float64(width-1))

	var b strings.Builder
	for i := 0; i < width; i++ {
		color := lerpColor(float64(i)/denom, ramp)
		b.WriteString(lipgloss.NewStyle().Foreground(color).Render("─"))
	}
	return b.String()
}

func renderSignalBar(width int, frame int) string {
	if width <= 0 {
		return ""
	}
	angle := float64(frame) * 0.14
	normalized := math.Asin(math.Sin(angle)) / (math.Pi / 2)
	head := (normalized + 1) / 2 * float64(max(1, width-1))
	spread := math.Max(3, float64(width)*0.18)
	flicker := 0.90 + 0.10*math.Sin(float64(frame)*0.37)

	var b strings.Builder
	for i := 0; i < width; i++ {
		dist := math.Abs(float64(i) - head)
		intensity := math.Exp(-(dist*dist)/(spread*spread)) * flicker
		intensity = clamp01(intensity)
		ch := "─"
		if intensity > 0.72 {
			ch = "━"
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lerpColor(intensity, coolRamp)).Render(ch))
	}
	return b.String()
}

func renderNeonProgressBar(percent float64, width int) string {
	width = clampInt(width, 1, 120)
	percent = math.Max(0, math.Min(100, percent))
	filled := int(math.Round(float64(width) * percent / 100.0))
	filled = clampInt(filled, 0, width)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			color := lerpColor(float64(i)/float64(max(1, width-1)), heatRamp)
			b.WriteString(lipgloss.NewStyle().Foreground(color).Render("█"))
		} else {
			b.WriteString(emptyBarStyle.Render("░"))
		}
	}
	return b.String()
}

func renderHalfHeightGradientBar(percent float64, width int) string {
	width = clampInt(width, 1, 120)
	percent = math.Max(0, math.Min(100, percent))
	filled := int(math.Round(float64(width) * percent / 100.0))
	filled = clampInt(filled, 0, width)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			color := lerpColor(float64(i)/float64(max(1, width-1)), heatRamp)
			b.WriteString(lipgloss.NewStyle().Foreground(color).Render("▄"))
		} else {
			b.WriteString(emptyBarStyle.Render("▁"))
		}
	}
	return b.String()
}

// ioRatePercent maps a byte/sec throughput onto a 0-100 scale logarithmically,
// so light traffic (KB/s) still produces a visible bar instead of rounding to
// empty against a high linear ceiling. ceiling is the rate that fills the bar.
func ioRatePercent(bytesPerSec float64, ceiling float64) float64 {
	const floor = 1024.0 // 1 KB/s — below this the bar reads as idle (empty)
	if bytesPerSec <= floor || ceiling <= floor {
		return 0
	}
	pct := math.Log10(bytesPerSec/floor) / math.Log10(ceiling/floor) * 100
	return math.Max(0, math.Min(100, pct))
}

func renderThinLineGraph(percent float64, width int) string {
	width = clampInt(width, 1, 120)
	percent = math.Max(0, math.Min(100, percent))
	filled := int(math.Round(float64(width) * percent / 100.0))
	filled = clampInt(filled, 0, width)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			color := lerpColor(float64(i)/float64(max(1, width-1)), heatRamp)
			b.WriteString(lipgloss.NewStyle().Foreground(color).Render("━"))
		} else {
			b.WriteString(emptyBarStyle.Render("─"))
		}
	}
	return b.String()
}

func renderSparkline(values []float64, width int) string {
	width = clampInt(width, 1, 120)
	if len(values) == 0 {
		return emptyBarStyle.Render(strings.Repeat("▁", width))
	}

	glyphs := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	samples := resampleValues(values, width)
	var b strings.Builder
	for _, value := range samples {
		value = math.Max(0, math.Min(100, value))
		idx := int(math.Round((value / 100) * float64(len(glyphs)-1)))
		idx = clampInt(idx, 0, len(glyphs)-1)
		b.WriteString(glyphs[idx])
	}
	return lipgloss.NewStyle().Foreground(cyanColor).Render(b.String())
}

// renderSparklineThreshold renders a trend like renderSparkline, but colors each
// point by its severity against t: samples below the warn level keep the themed
// accent, while points that crossed warn/crit are drawn amber/red so a past
// spike is visible at the moment in time it happened. A disabled (0/0) threshold
// renders identically to renderSparkline.
func renderSparklineThreshold(values []float64, width int, t Threshold) string {
	width = clampInt(width, 1, 120)
	if len(values) == 0 {
		return emptyBarStyle.Render(strings.Repeat("▁", width))
	}

	glyphs := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	base := lipgloss.NewStyle().Foreground(cyanColor)
	samples := resampleValues(values, width)

	var b strings.Builder
	// Coalesce consecutive same-severity points into one styled run to keep the
	// escape-sequence count down.
	runStart := 0
	for i := 0; i <= len(samples); i++ {
		if i < len(samples) && severityFor(samples[i], t) == severityFor(samples[runStart], t) {
			continue
		}
		var run strings.Builder
		for j := runStart; j < i; j++ {
			v := math.Max(0, math.Min(100, samples[j]))
			idx := clampInt(int(math.Round((v/100)*float64(len(glyphs)-1))), 0, len(glyphs)-1)
			run.WriteString(glyphs[idx])
		}
		b.WriteString(severityStyle(severityFor(samples[runStart], t), base).Render(run.String()))
		runStart = i
	}
	return b.String()
}

func resampleValues(values []float64, width int) []float64 {
	if len(values) >= width {
		return values[len(values)-width:]
	}
	samples := make([]float64, width)
	pad := width - len(values)
	for i := 0; i < pad; i++ {
		samples[i] = values[0]
	}
	copy(samples[pad:], values)
	return samples
}

func renderPanel(title string, body string, width int) string {
	width = max(16, width)
	innerWidth := width - 2
	titleText := " " + title + " "
	if lipgloss.Width(titleText) > innerWidth {
		titleText = " " + truncateVisible(title, innerWidth-2) + " "
	}
	ruleWidth := innerWidth - lipgloss.Width(titleText)
	leftRule := ruleWidth / 2
	rightRule := ruleWidth - leftRule

	top := panelBorderStyle.Render("╭"+strings.Repeat("─", leftRule)) +
		panelTitleStyle.Render(titleText) +
		panelBorderStyle.Render(strings.Repeat("─", rightRule)+"╮")

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	var b strings.Builder
	b.WriteString(top)
	for _, line := range lines {
		visible := lipgloss.Width(line)
		if visible > innerWidth {
			line = truncateVisible(line, innerWidth)
			visible = lipgloss.Width(line)
		}
		b.WriteString("\n")
		b.WriteString(panelBorderStyle.Render("│"))
		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", innerWidth-visible))
		b.WriteString(panelBorderStyle.Render("│"))
	}
	b.WriteString("\n")
	b.WriteString(panelBorderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯"))
	return b.String()
}

func renderHeroHeader(title string, subtitle string, width int, frame int) string {
	width = max(40, width)
	contentWidth := width - 2
	var b strings.Builder
	b.WriteString(renderGradientText(title, coolRamp))
	if subtitle != "" {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(subtitle))
	}
	b.WriteString("\n")
	b.WriteString(renderSignalBar(contentWidth, frame))
	return b.String()
}

func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	for _, r := range runes {
		if lipgloss.Width(b.String()+string(r)) > width-1 {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}

func abbreviateCPUModel(model string, maxWidth int) string {
	s := model
	s = strings.ReplaceAll(s, "Intel(R) ", "")
	s = strings.ReplaceAll(s, "Core(TM) ", "")
	s = strings.ReplaceAll(s, " CPU", "")
	s = strings.ReplaceAll(s, "GenuineIntel", "Intel")
	s = strings.ReplaceAll(s, "AuthenticAMD", "AMD")
	s = strings.ReplaceAll(s, "with Radeon", "")
	s = strings.ReplaceAll(s, " Graphics", "")
	s = strings.ReplaceAll(s, "Processor", "")
	s = strings.ReplaceAll(s, "Six-Core", "6c")
	s = strings.ReplaceAll(s, "Eight-Core", "8c")
	s = strings.ReplaceAll(s, "12-Core", "12c")
	s = strings.ReplaceAll(s, "16-Core", "16c")
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	return truncateVisible(s, maxWidth)
}

func metricBarWidth(totalWidth int) int {
	return clampInt(totalWidth-8, 18, 56)
}
