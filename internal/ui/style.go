package ui

// A Style decides how things are drawn (panel chrome, header, gauges); a Theme
// decides what color they are. The two are independent, so every palette works
// under every style.

const (
	styleClassic     = "classic"
	styleDrift       = "drift"
	defaultStyleName = styleClassic

	// marThemeName is the palette drift was designed around. Switching to drift
	// from the stock palette adopts it, and switching back restores the stock one.
	marThemeName = "mar"

	// styleToastFrames is how many animation ticks the "style: x" confirmation
	// stays in the header after a switch.
	styleToastFrames = 40
)

// Style bundles the drawing routines that differ between looks.
type Style struct {
	Name  string
	Blurb string
	// Panel draws a titled section, width columns wide and body+2 lines tall.
	Panel func(title, body string, width int, sev Severity) string
	// Hero draws the three-line screen header.
	Hero func(title, subtitle string, width, frame int) string
	// Bar draws a value gauge. full/empty are the glyphs the caller would use in
	// the classic look; styles are free to ignore them.
	Bar func(percent float64, width int, t Threshold, full, empty string) string
}

// activeStyle is the style the non-method render helpers draw with. View()
// publishes the configured style here each frame, like activeFrame.
var (
	activeStyle = classicStyle()
	// activeToast is a short confirmation shown in the header after a switch.
	activeToast string
)

// BuiltInStyles lists the available styles in cycling order.
func BuiltInStyles() []Style {
	return []Style{classicStyle(), driftStyle()}
}

func classicStyle() Style {
	return Style{
		Name:  styleClassic,
		Blurb: "boxed neon panels",
		Panel: classicPanel,
		Hero:  classicHero,
		Bar:   classicBar,
	}
}

// StyleByName returns the named style, or classic for an unknown name.
func StyleByName(name string) Style {
	for _, s := range BuiltInStyles() {
		if s.Name == name {
			return s
		}
	}
	return classicStyle()
}

// StyleNames lists style names in cycling order.
func StyleNames() []string {
	styles := BuiltInStyles()
	names := make([]string, len(styles))
	for i, s := range styles {
		names[i] = s.Name
	}
	return names
}

func nextStyleName(current string, delta int) string {
	names := StyleNames()
	idx := 0
	for i, name := range names {
		if name == current {
			idx = i
			break
		}
	}
	return names[(idx+delta+len(names))%len(names)]
}

// cycleStyle moves to the next style, persists it, and flashes a confirmation.
// Moving between the stock palette and drift's own palette swaps them so the
// switch reads as a whole new look; any other palette the user picked is kept.
func (m *Model) cycleStyle() {
	m.setStyle(nextStyleName(m.settings.Style, 1))
}

func (m *Model) setStyle(name string) {
	prev := m.settings.Style
	m.settings.Style = StyleByName(name).Name
	switch {
	case m.settings.Style == styleDrift && m.settings.DefaultTheme == defaultThemeName:
		m.settings.DefaultTheme = marThemeName
	case prev == styleDrift && m.settings.Style != styleDrift && m.settings.DefaultTheme == marThemeName:
		m.settings.DefaultTheme = defaultThemeName
	}
	m.styleToast = "style: " + m.settings.Style
	m.styleToastUntil = m.animationFrame + styleToastFrames
	_ = SaveSettings(m.settings)
}
