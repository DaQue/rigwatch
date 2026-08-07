package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// thresholdRef names a metric's threshold pair and locates it inside a
// Thresholds value, so the settings form can build inputs and write them back
// without a hand-maintained switch per metric.
type thresholdRef struct {
	label string
	get   func(*Thresholds) *Threshold
}

func thresholdRefs() []thresholdRef {
	return []thresholdRef{
		{"CPU %", func(t *Thresholds) *Threshold { return &t.CPUPct }},
		{"RAM %", func(t *Thresholds) *Threshold { return &t.RAMPct }},
		{"Swap %", func(t *Thresholds) *Threshold { return &t.SwapPct }},
		{"Disk %", func(t *Thresholds) *Threshold { return &t.DiskPct }},
		{"Temp °C", func(t *Thresholds) *Threshold { return &t.TempC }},
		{"GPU temp °C", func(t *Thresholds) *Threshold { return &t.GPUTempC }},
		{"GPU util %", func(t *Thresholds) *Threshold { return &t.GPUPct }},
	}
}

// settingsFormState is the editable state of the settings screen. Focus 0 is the
// theme cycler and focus 1 the headline cycler; focus 2 is the interval input;
// the remaining focuses are the warn/crit inputs (two per threshold metric), in
// thresholdRefs() order.
type settingsFormState struct {
	inputs      []textinput.Model // [0]=interval, then warn,crit per metric
	themeIdx    int
	headlineIdx int
	focus       int
	status      string
	statusErr   bool
}

func numericInput(value string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.CharLimit = 7
	ti.Width = 7
	ti.Prompt = ""
	return ti
}

// settingsCyclerRows is how many leading focus stops are ←/→ cyclers rather than
// text inputs: the theme picker and the headline picker.
const settingsCyclerRows = 2

// focusCount is the number of focus stops: the cyclers + interval + 2 per metric.
func (s *settingsFormState) focusCount() int { return settingsCyclerRows + len(s.inputs) }

// inputIndexFor maps a focus index to an inputs[] index, or -1 for a cycler row.
func inputIndexFor(focus int) int {
	if focus < settingsCyclerRows {
		return -1
	}
	return focus - settingsCyclerRows
}

func formatThreshold(v float64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (m *Model) openSettings() {
	refs := thresholdRefs()
	inputs := make([]textinput.Model, 0, 1+len(refs)*2)

	intervalVal := ""
	if m.settings.Interval > 0 {
		intervalVal = strconv.FormatFloat(m.settings.Interval, 'f', -1, 64)
	}
	inputs = append(inputs, numericInput(intervalVal))

	th := m.settings.Thresholds
	for _, ref := range refs {
		pair := ref.get(&th)
		inputs = append(inputs, numericInput(formatThreshold(pair.Warn)))
		inputs = append(inputs, numericInput(formatThreshold(pair.Crit)))
	}

	themeIdx := 0
	for i, name := range ThemeNames() {
		if name == m.settings.DefaultTheme {
			themeIdx = i
			break
		}
	}

	headlineIdx := 0
	for i, mode := range HeadlineModes() {
		if mode == m.settings.HeadlineMode {
			headlineIdx = i
			break
		}
	}

	m.settingsForm = &settingsFormState{inputs: inputs, themeIdx: themeIdx, headlineIdx: headlineIdx, focus: 0}
	m.screen = ScreenSettings
	m.setSettingsFocus(0)
}

func (m *Model) setSettingsFocus(i int) {
	s := m.settingsForm
	count := s.focusCount()
	if i < 0 {
		i = count - 1
	}
	if i >= count {
		i = 0
	}
	s.focus = i
	for j := range s.inputs {
		s.inputs[j].Blur()
	}
	if idx := inputIndexFor(i); idx >= 0 {
		s.inputs[idx].Focus()
	}
}

// updateSettings owns all key input while the settings screen is active.
func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := m.settingsForm
	if s == nil {
		m.screen = ScreenHostList
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.settingsForm = nil
		m.screen = ScreenHostList
		return m, nil
	case "enter", "ctrl+s":
		return m.saveSettingsForm()
	case "tab", "down":
		m.setSettingsFocus(s.focus + 1)
		return m, textinput.Blink
	case "shift+tab", "up":
		m.setSettingsFocus(s.focus - 1)
		return m, textinput.Blink
	case "left":
		switch s.focus {
		case 0:
			s.themeIdx = (s.themeIdx - 1 + len(ThemeNames())) % len(ThemeNames())
			return m, nil
		case 1:
			s.headlineIdx = (s.headlineIdx - 1 + len(HeadlineModes())) % len(HeadlineModes())
			return m, nil
		}
	case "right":
		switch s.focus {
		case 0:
			s.themeIdx = (s.themeIdx + 1) % len(ThemeNames())
			return m, nil
		case 1:
			s.headlineIdx = (s.headlineIdx + 1) % len(HeadlineModes())
			return m, nil
		}
	}

	// Forward everything else to the focused numeric input (theme row has none).
	if idx := inputIndexFor(s.focus); idx >= 0 {
		var cmd tea.Cmd
		s.inputs[idx], cmd = s.inputs[idx].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) saveSettingsForm() (tea.Model, tea.Cmd) {
	s := m.settingsForm

	interval, err := parseOptionalFloat(s.inputs[0].Value())
	if err != nil {
		s.status = "Interval must be a number (seconds), or blank for the default"
		s.statusErr = true
		return m, nil
	}

	refs := thresholdRefs()
	var thresholds Thresholds
	for i, ref := range refs {
		warn, werr := parseOptionalFloat(s.inputs[1+i*2].Value())
		crit, cerr := parseOptionalFloat(s.inputs[2+i*2].Value())
		if werr != nil || cerr != nil {
			s.status = ref.label + " thresholds must be numbers, or blank to disable"
			s.statusErr = true
			return m, nil
		}
		*ref.get(&thresholds) = Threshold{Warn: warn, Crit: crit}
	}

	newSettings := Settings{
		Interval:                 interval,
		DefaultTheme:             ThemeNames()[s.themeIdx],
		Thresholds:               thresholds,
		HeadlineMode:             HeadlineModes()[s.headlineIdx],
		ShowExtendedPanels:       m.settings.ShowExtendedPanels,
		CheckForUpdates:          m.settings.CheckForUpdates,
		UpdateCheckIntervalHours: m.settings.UpdateCheckIntervalHours,
	}
	if err := SaveSettings(newSettings); err != nil {
		s.status = "Save failed: " + err.Error()
		s.statusErr = true
		return m, nil
	}

	// Apply live: thresholds take effect on next render via View(); interval
	// applies to the next tick.
	m.settings = newSettings
	activeThresholds = newSettings.Thresholds
	if interval > 0 {
		m.updateInterval = time.Duration(interval * float64(time.Second))
	}
	m.settingsForm = nil
	m.screen = ScreenHostList
	m.updateListSelection()
	return m, nil
}

func parseOptionalFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return v, nil
}

func (m Model) renderSettingsScreen() string {
	s := m.settingsForm
	if s == nil {
		return ""
	}

	var b strings.Builder
	subtitle := "↑/↓ move  •  ←/→ change option  •  type to edit  •  enter save  •  esc cancel"
	b.WriteString(renderHeroHeader("RIGWATCH // SETTINGS", subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")

	focusStyle := lipgloss.NewStyle().Foreground(cyanColor).Bold(true)
	label := func(text string, focused bool) string {
		marker := "  "
		if focused {
			marker = focusStyle.Render("▸ ")
		}
		return marker + fmt.Sprintf("%-16s", text)
	}

	// Theme row (focus 0).
	themeName := ThemeNames()[s.themeIdx]
	b.WriteString(label("Default theme", s.focus == 0))
	b.WriteString(accentStyle.Render("‹ " + themeName + " ›"))
	b.WriteString("\n")

	// Headline row (focus 1).
	b.WriteString(label("Tile headline", s.focus == 1))
	b.WriteString(accentStyle.Render("‹ " + HeadlineModes()[s.headlineIdx] + " ›"))
	b.WriteString(mutedStyle.Render("  big alert reading per tile"))
	b.WriteString("\n")

	// Interval row (focus 2 / inputs[0]).
	b.WriteString(label("Refresh interval", s.focus == 2))
	b.WriteString(s.inputs[0].View())
	b.WriteString(mutedStyle.Render(" sec (blank = default)"))
	b.WriteString("\n\n")

	b.WriteString(mutedStyle.Render("  THRESHOLDS                warn     crit"))
	b.WriteString("\n")
	refs := thresholdRefs()
	for i, ref := range refs {
		warnFocus := s.focus == 3+i*2
		critFocus := s.focus == 4+i*2
		b.WriteString(label(ref.label, warnFocus || critFocus))
		b.WriteString(s.inputs[1+i*2].View())
		b.WriteString("  ")
		b.WriteString(s.inputs[2+i*2].View())
		b.WriteString("\n")
	}

	if s.status != "" {
		b.WriteString("\n")
		style := successStyle
		if s.statusErr {
			style = dangerStyle
		}
		b.WriteString(style.Render(s.status))
	}

	out := b.String()
	if m.height > 0 {
		out = clampToHeight(out, m.height)
	}
	return out
}
