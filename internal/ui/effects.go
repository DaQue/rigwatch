package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// activeFrame is the animation tick the non-method render helpers read, mirroring
// how activeThresholds exposes the live settings. Model.View() refreshes it each
// frame so panels can animate without threading a frame argument through every
// render signature.
var activeFrame int

// trueColorAvailable reports whether the terminal can render the 24-bit colors
// the panel fill and pulse depend on. On 256-color and monochrome profiles those
// effects quantize into mud, so they are skipped rather than approximated.
func trueColorAvailable() bool {
	return lipgloss.ColorProfile() == termenv.TrueColor
}

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

// warnRampPos and critRampPos are where a metric's warn and crit levels land on
// heatRamp. Warn sits just past the ramp's yellow midpoint and crit near its red
// end, so the same color means the same thing on every metric.
const (
	warnRampPos = 0.55
	critRampPos = 0.85
)

// heatRampPos maps a 0-100 reading onto a heatRamp position so bar color encodes
// severity rather than bar length. With a configured threshold pair the warn
// level lands in the amber band and crit in the red one, which keeps bar color
// consistent with severityFor and the alert panel. A disabled (0/0) pair falls
// back to the raw value, so an unthresholded metric still reads hotter as it
// climbs.
func heatRampPos(value float64, t Threshold) float64 {
	v := clamp01(value / 100)
	warn, crit := t.Warn/100, t.Crit/100
	if warn <= 0 || crit <= warn {
		return v
	}
	switch {
	case v < warn:
		return v / warn * warnRampPos
	case v < crit:
		return warnRampPos + (v-warn)/(crit-warn)*(critRampPos-warnRampPos)
	case crit >= 1:
		return 1
	default:
		return critRampPos + (v-crit)/(1-crit)*(1-critRampPos)
	}
}

// renderValueBar draws a filled/empty bar whose gradient is anchored to the
// reading's severity position instead of to the bar's own length. The filled run
// sweeps from the ramp's cool end up to that position, so the bar's leading edge
// is always the color that corresponds to the current value — a bar sitting at
// 30% is green whether it is 8 cells wide or 40.
func renderValueBar(percent float64, width int, t Threshold, fullGlyph, emptyGlyph string) string {
	width = clampInt(width, 1, 120)
	percent = math.Max(0, math.Min(100, percent))
	filled := clampInt(int(math.Round(float64(width)*percent/100.0)), 0, width)
	tip := heatRampPos(percent, t)

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i >= filled {
			b.WriteString(emptyBarStyle.Render(emptyGlyph))
			continue
		}
		frac := 1.0
		if filled > 1 {
			frac = float64(i) / float64(filled-1)
		}
		b.WriteString(lipgloss.NewStyle().Foreground(lerpColor(tip*frac, heatRamp)).Render(fullGlyph))
	}
	return b.String()
}

func renderNeonProgressBar(percent float64, width int) string {
	return renderNeonProgressBarSev(percent, width, Threshold{})
}

func renderNeonProgressBarSev(percent float64, width int, t Threshold) string {
	return renderValueBar(percent, width, t, "█", "░")
}

func renderHalfHeightGradientBar(percent float64, width int) string {
	return renderHalfHeightGradientBarSev(percent, width, Threshold{})
}

func renderHalfHeightGradientBarSev(percent float64, width int, t Threshold) string {
	return renderValueBar(percent, width, t, "▄", "▁")
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
	return renderThinLineGraphSev(percent, width, Threshold{})
}

func renderThinLineGraphSev(percent float64, width int, t Threshold) string {
	return renderValueBar(percent, width, t, "━", "─")
}

// Braille dot bits for the left and right sub-columns of a U+2800 cell, ordered
// bottom-to-top. A braille cell is a 2x4 dot matrix, so one terminal cell holds
// two samples at four vertical levels — double the horizontal trend resolution
// of the block glyphs, in the same width.
var (
	brailleLeftBits  = [4]rune{0x40, 0x04, 0x02, 0x01}
	brailleRightBits = [4]rune{0x80, 0x20, 0x10, 0x08}
)

// brailleBaseline is the flat "floor" glyph (bottom dots only) used for an idle
// or empty trend, so a quiet metric still shows an axis instead of blank space.
const brailleBaseline = "⣀"

// brailleLevel quantizes a 0-100 reading to filled dots across rows text rows,
// giving 4*rows levels. A non-zero reading never rounds down to nothing, so
// light activity stays visible instead of reading as idle.
func brailleLevel(value float64, rows int) int {
	value = math.Max(0, math.Min(100, value))
	steps := 4 * rows
	level := int(math.Round(value / 100 * float64(steps)))
	if level == 0 && value > 0 {
		level = 1
	}
	return clampInt(level, 0, steps)
}

// brailleColumnDots is how many of a given text row's 4 dots a level fills, with
// row 0 the topmost. Rows fill bottom-up, so the bottom row saturates first.
func brailleColumnDots(level, row, rows int) int {
	fromBottom := (rows - 1 - row) * 4
	return clampInt(level-fromBottom, 0, 4)
}

// brailleCell packs two samples into one glyph for the given text row, left
// sample in the left dot column, drawn as bars rising from the bottom row's edge.
func brailleCell(left, right float64, row, rows int) string {
	cell := rune(0x2800)
	for i := 0; i < brailleColumnDots(brailleLevel(left, rows), row, rows); i++ {
		cell |= brailleLeftBits[i]
	}
	for i := 0; i < brailleColumnDots(brailleLevel(right, rows), row, rows); i++ {
		cell |= brailleRightBits[i]
	}
	return string(cell)
}

func renderSparkline(values []float64, width int) string {
	return renderSparklineThreshold(values, width, Threshold{})
}

func renderSparklineThreshold(values []float64, width int, t Threshold) string {
	return renderSparklineRows(values, width, 1, t)
}

// renderSparklineRows renders a braille trend across rows text rows, coloring
// each cell by its severity against t: cells below the warn level keep the themed
// accent, while cells holding a sample that crossed warn/crit are drawn
// amber/red, so a past spike stays visible at the moment in time it happened. A
// disabled (0/0) threshold renders as a single accent-colored run.
//
// Braille packs two samples per cell, so the trend shows 2*width points of
// history. Vertical resolution is 4 levels per row: one row trades half the
// vertical detail of the old block glyphs for double the horizontal detail, while
// two rows keep all 8 levels and still double the horizontal detail.
func renderSparklineRows(values []float64, width, rows int, t Threshold) string {
	width = clampInt(width, 1, 120)
	rows = clampInt(rows, 1, 4)
	if len(values) == 0 {
		return strings.Repeat(emptyBarStyle.Render(strings.Repeat(brailleBaseline, width))+"\n", rows-1) +
			emptyBarStyle.Render(strings.Repeat(brailleBaseline, width))
	}

	samples := resampleValues(values, width*2)
	base := lipgloss.NewStyle().Foreground(cyanColor)

	cellSeverity := func(cell int) Severity {
		left, right := severityFor(samples[cell*2], t), severityFor(samples[cell*2+1], t)
		if right > left {
			return right
		}
		return left
	}

	lines := make([]string, rows)
	for row := 0; row < rows; row++ {
		var b strings.Builder
		// Coalesce consecutive same-severity cells into one styled run to keep the
		// escape-sequence count down.
		runStart := 0
		for i := 0; i <= width; i++ {
			if i < width && cellSeverity(i) == cellSeverity(runStart) {
				continue
			}
			var run strings.Builder
			for j := runStart; j < i; j++ {
				run.WriteString(brailleCell(samples[j*2], samples[j*2+1], row, rows))
			}
			b.WriteString(severityStyle(cellSeverity(runStart), base).Render(run.String()))
			runStart = i
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// renderLabeledTrend renders a trend behind a label, indenting any continuation
// rows to stay aligned under the first one.
func renderLabeledTrend(label string, values []float64, width, rows int, t Threshold) string {
	trend := strings.Split(renderSparklineRows(values, width, rows, t), "\n")
	indent := strings.Repeat(" ", lipgloss.Width(label))
	var b strings.Builder
	for i, line := range trend {
		if i > 0 {
			b.WriteString("\n")
			b.WriteString(indent)
		} else {
			b.WriteString(mutedStyle.Render(label))
		}
		b.WriteString(line)
	}
	return b.String()
}

// trendRows is how many text rows a trend gets: the roomy single-host view keeps
// the full 8 vertical levels, while the compact grid stays one row per trend so
// the tiles' panel stack still fits.
func trendRows(extended bool) int {
	if extended {
		return 2
	}
	return 1
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

// pulseColor dims c on a slow sine so a critical panel breathes instead of
// sitting static. The 0.62 floor keeps the border legible at the dim end of the
// cycle, and the effect is dropped entirely without true color, where the dimmed
// steps would all quantize back to the same palette entry.
func pulseColor(c lipgloss.Color, frame int) lipgloss.Color {
	hex := string(c)
	if len(hex) != 7 || hex[0] != '#' || !trueColorAvailable() {
		return c
	}
	f := 0.62 + 0.38*(0.5+0.5*math.Sin(float64(frame)*0.42))
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		uint8(hexRed(hex)*f), uint8(hexGreen(hex)*f), uint8(hexBlue(hex)*f)))
}

// panelFillSGR is the raw background escape for the panel interior. It is raw
// rather than a lipgloss style because body lines already carry their own color
// codes, and every one of those ends in a full reset that would drop the
// background partway along the line; applyPanelFill re-arms it after each reset.
func panelFillSGR() string {
	if !trueColorAvailable() {
		return ""
	}
	hex := string(panelBgColor)
	if len(hex) != 7 || hex[0] != '#' {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", int(hexRed(hex)), int(hexGreen(hex)), int(hexBlue(hex)))
}

func applyPanelFill(line, fill string) string {
	if fill == "" {
		return line
	}
	return fill + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+fill) + "\x1b[0m"
}

func renderPanel(title string, body string, width int) string {
	return renderPanelSev(title, body, width, SevOK)
}

// renderPanelSev draws a panel whose border and title carry sev, so that on a
// screen of a dozen identically shaped panels the one with an active alert is
// the one that catches the eye. A critical panel also pulses via activeFrame.
// The interior is filled with the theme's panel background, which lifts the card
// off the terminal background instead of leaving everything on one flat plane.
func renderPanelSev(title string, body string, width int, sev Severity) string {
	width = max(16, width)
	innerWidth := width - 2

	borderStyle, titleStyle := panelBorderStyle, panelTitleStyle
	switch sev {
	case SevWarn:
		borderStyle = lipgloss.NewStyle().Foreground(warningColor).Bold(true)
		titleStyle = borderStyle
	case SevCrit:
		pulsed := pulseColor(dangerColor, activeFrame)
		borderStyle = lipgloss.NewStyle().Foreground(pulsed).Bold(true)
		titleStyle = borderStyle
	}

	titleText := " " + severityMarker(sev) + title + " "
	if lipgloss.Width(titleText) > innerWidth {
		titleText = " " + truncateVisible(severityMarker(sev)+title, innerWidth-2) + " "
	}
	ruleWidth := innerWidth - lipgloss.Width(titleText)
	leftRule := ruleWidth / 2
	rightRule := ruleWidth - leftRule

	top := borderStyle.Render("╭"+strings.Repeat("─", leftRule)) +
		titleStyle.Render(titleText) +
		borderStyle.Render(strings.Repeat("─", rightRule)+"╮")

	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	fill := panelFillSGR()
	var b strings.Builder
	b.WriteString(top)
	for _, line := range lines {
		visible := lipgloss.Width(line)
		if visible > innerWidth {
			line = truncateVisible(line, innerWidth)
			visible = lipgloss.Width(line)
		}
		b.WriteString("\n")
		b.WriteString(borderStyle.Render("│"))
		b.WriteString(applyPanelFill(line+strings.Repeat(" ", innerWidth-visible), fill))
		b.WriteString(borderStyle.Render("│"))
	}
	b.WriteString("\n")
	b.WriteString(borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯"))
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
