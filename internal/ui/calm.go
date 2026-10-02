package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// calmDim is how far a healthy host's colors are pulled toward its background in
// calm mode: 0 leaves them alone, 1 erases them. Enough that a wall of healthy
// tiles recedes into the background, little enough that the numbers stay legible
// when you do look.
const calmDim = 0.58

// calmTheme returns theme with every color pulled toward its panel background.
// Danger and warning colors are dimmed too, but they only reach the screen on a
// host that has alerts, and those hosts are never rendered calm.
func calmTheme(theme Theme) Theme {
	bg := string(theme.PanelBg)
	dim := func(c lipgloss.Color) lipgloss.Color { return blendHex(string(c), bg, calmDim) }
	dimRamp := func(ramp []colorStop) []colorStop {
		out := make([]colorStop, len(ramp))
		bgR, bgG, bgB := hexRed(bg), hexGreen(bg), hexBlue(bg)
		for i, s := range ramp {
			out[i] = colorStop{
				pos:   s.pos,
				red:   s.red + (bgR-s.red)*calmDim,
				green: s.green + (bgG-s.green)*calmDim,
				blue:  s.blue + (bgB-s.blue)*calmDim,
			}
		}
		return out
	}
	theme.Ink = dim(theme.Ink)
	theme.Muted = dim(theme.Muted)
	theme.Accent = dim(theme.Accent)
	theme.Success = dim(theme.Success)
	theme.Warning = dim(theme.Warning)
	theme.Danger = dim(theme.Danger)
	theme.Border = dim(theme.Border)
	theme.Title = dim(theme.Title)
	theme.FocusColor = dim(theme.FocusColor)
	theme.CoolRamp = dimRamp(theme.CoolRamp)
	theme.HeatRamp = dimRamp(theme.HeatRamp)
	return theme
}

// blendHex mixes color a toward color b by t (0 = a, 1 = b). Inputs are #rrggbb;
// anything else is returned unchanged.
func blendHex(a, b string, t float64) lipgloss.Color {
	if len(a) != 7 || len(b) != 7 || a[0] != '#' || b[0] != '#' {
		return lipgloss.Color(a)
	}
	mix := func(x, y float64) int { return int(x + (y-x)*t + 0.5) }
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x",
		mix(hexRed(a), hexRed(b)), mix(hexGreen(a), hexGreen(b)), mix(hexBlue(a), hexBlue(b))))
}

// toggleCalmMode flips calm mode and persists it. A failed save is not worth
// interrupting the dashboard for; the mode still applies for this session.
func (m *Model) toggleCalmMode() {
	m.settings.CalmMode = !m.settings.CalmMode
	_ = SaveSettings(m.settings)
}
