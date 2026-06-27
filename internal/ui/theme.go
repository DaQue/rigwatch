package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

const defaultThemeName = "rigwatch"

type Theme struct {
	Name       string
	Ink        lipgloss.Color
	Muted      lipgloss.Color
	Accent     lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Danger     lipgloss.Color
	PanelBg    lipgloss.Color
	PanelDim   lipgloss.Color
	Border     lipgloss.Color
	Title      lipgloss.Color
	TitleBg    lipgloss.Color
	CoolRamp   []colorStop
	HeatRamp   []colorStop
	FocusColor lipgloss.Color
}

type ThemePreferences struct {
	Hosts map[string]string `json:"hosts"`
}

func BuiltInThemes() []Theme {
	return []Theme{
		{
			Name: defaultThemeName, Ink: lipgloss.Color("#f8fbff"), Muted: lipgloss.Color("#6f7a99"),
			Accent: lipgloss.Color("#32d9ff"), Success: lipgloss.Color("#27ef9f"), Warning: lipgloss.Color("#ffa62b"), Danger: lipgloss.Color("#ff3b6b"),
			PanelBg: lipgloss.Color("#050711"), PanelDim: lipgloss.Color("#202642"), Border: lipgloss.Color("#7c3aff"), Title: lipgloss.Color("#ff4fc3"), TitleBg: lipgloss.Color("#15112b"), FocusColor: lipgloss.Color("#32d9ff"),
			CoolRamp: cloneRamp(coolRamp), HeatRamp: cloneRamp(heatRamp),
		},
		makeTheme("matrix-pulse", "#d9ffe8", "#5f8f72", "#00ff87", "#3dff9f", "#c8ff4d", "#ff4f6a", "#031007", "#173321", "#00a86b", "#7cffb2", "#052114"),
		makeTheme("ember-ops", "#fff8ee", "#95705f", "#ff8a3d", "#ffbf5f", "#ffd15c", "#ff4d2e", "#140705", "#402018", "#d95c25", "#ffb36b", "#2b1008"),
		makeTheme("arctic-scan", "#f4fbff", "#728a99", "#6ee7ff", "#7dd3fc", "#fde68a", "#fb7185", "#031019", "#183240", "#38bdf8", "#bae6fd", "#082030"),
		makeTheme("amber-crt", "#fff4d6", "#9f8251", "#ffbf3f", "#ffd166", "#f4e06d", "#ff5f3f", "#120d03", "#3c2f12", "#c9972d", "#ffe08a", "#241803"),
		makeTheme("dracula", "#f8f8f2", "#6272a4", "#8be9fd", "#50fa7b", "#f1fa8c", "#ff5555", "#191a21", "#343746", "#bd93f9", "#ff79c6", "#282a36"),
		makeTheme("nord", "#eceff4", "#81a1c1", "#88c0d0", "#a3be8c", "#ebcb8b", "#bf616a", "#121821", "#2e3440", "#5e81ac", "#b48ead", "#1d2530"),
		makeTheme("gruvbox-dark", "#ebdbb2", "#928374", "#83a598", "#b8bb26", "#fabd2f", "#fb4934", "#1d2021", "#3c3836", "#d3869b", "#fe8019", "#282828"),
		makeTheme("solarized-dark", "#eee8d5", "#839496", "#2aa198", "#859900", "#b58900", "#dc322f", "#002b36", "#073642", "#268bd2", "#d33682", "#073642"),
		makeTheme("catppuccin-mocha", "#cdd6f4", "#7f849c", "#89dceb", "#a6e3a1", "#f9e2af", "#f38ba8", "#11111b", "#313244", "#cba6f7", "#f5c2e7", "#1e1e2e"),
	}
}

func makeTheme(name, ink, muted, accent, success, warning, danger, panelBg, panelDim, border, title, titleBg string) Theme {
	return Theme{
		Name: name, Ink: lipgloss.Color(ink), Muted: lipgloss.Color(muted), Accent: lipgloss.Color(accent),
		Success: lipgloss.Color(success), Warning: lipgloss.Color(warning), Danger: lipgloss.Color(danger),
		PanelBg: lipgloss.Color(panelBg), PanelDim: lipgloss.Color(panelDim), Border: lipgloss.Color(border), Title: lipgloss.Color(title), TitleBg: lipgloss.Color(titleBg), FocusColor: lipgloss.Color(accent),
		CoolRamp: []colorStop{
			{pos: 0.00, red: hexRed(border), green: hexGreen(border), blue: hexBlue(border)},
			{pos: 0.50, red: hexRed(title), green: hexGreen(title), blue: hexBlue(title)},
			{pos: 1.00, red: hexRed(ink), green: hexGreen(ink), blue: hexBlue(ink)},
		},
		HeatRamp: cloneRamp(heatRamp),
	}
}

func ThemeByName(name string) Theme {
	for _, theme := range BuiltInThemes() {
		if theme.Name == name {
			return theme
		}
	}
	return BuiltInThemes()[0]
}

func ThemeNames() []string {
	themes := BuiltInThemes()
	names := make([]string, len(themes))
	for i, theme := range themes {
		names[i] = theme.Name
	}
	return names
}

func nextThemeName(current string, delta int) string {
	names := ThemeNames()
	idx := 0
	for i, name := range names {
		if name == current {
			idx = i
			break
		}
	}
	return names[(idx+delta+len(names))%len(names)]
}

func LoadThemePreferences() (ThemePreferences, error) {
	path, err := themePreferencesPath()
	if err != nil {
		return ThemePreferences{Hosts: make(map[string]string)}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ThemePreferences{Hosts: make(map[string]string)}, nil
	}
	if err != nil {
		return ThemePreferences{Hosts: make(map[string]string)}, err
	}
	var prefs ThemePreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return ThemePreferences{Hosts: make(map[string]string)}, err
	}
	if prefs.Hosts == nil {
		prefs.Hosts = make(map[string]string)
	}
	return prefs, nil
}

func SaveThemePreferences(prefs ThemePreferences) error {
	path, err := themePreferencesPath()
	if err != nil {
		return err
	}
	if prefs.Hosts == nil {
		prefs.Hosts = make(map[string]string)
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func themePreferencesPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if configDir == "" {
		return "", fmt.Errorf("user config directory unavailable")
	}
	return filepath.Join(configDir, "rigwatch", "themes.json"), nil
}

func (p ThemePreferences) ThemeNameForHost(hostName string) string {
	if p.Hosts == nil {
		return defaultThemeName
	}
	if name := p.Hosts[hostName]; name != "" {
		return ThemeByName(name).Name
	}
	return defaultThemeName
}

func (p *ThemePreferences) SetHostTheme(hostName, themeName string) {
	if p.Hosts == nil {
		p.Hosts = make(map[string]string)
	}
	p.Hosts[hostName] = ThemeByName(themeName).Name
}

func (m Model) themeForHost(hostName string) Theme {
	return ThemeByName(m.themePrefs.ThemeNameForHost(hostName))
}

func (m Model) renderHostThemed(hostName string, render func() string) string {
	return renderWithTheme(m.themeForHost(hostName), render)
}

func (m Model) renderThemedHostPanel(host internal.SSHHost, focused bool, body string, width int) string {
	theme := m.themeForHost(host.Name)
	title := "◈ " + host.Name
	if focused {
		title += " [" + theme.Name + "]"
	}
	return renderWithTheme(theme, func() string {
		if !focused {
			return renderPanel(title, body, width)
		}
		originalBorder := panelBorderStyle
		originalTitle := panelTitleStyle
		panelBorderStyle = lipgloss.NewStyle().Foreground(theme.FocusColor).Bold(true)
		panelTitleStyle = lipgloss.NewStyle().Foreground(theme.FocusColor).Bold(true)
		defer func() {
			panelBorderStyle = originalBorder
			panelTitleStyle = originalTitle
		}()
		return renderPanel(title, body, width)
	})
}

func renderWithTheme(theme Theme, render func() string) string {
	snapshot := captureThemeState()
	applyTheme(theme)
	defer restoreThemeState(snapshot)
	return render()
}

type themeState struct {
	inkColor, mutedColor, violetColor, magentaColor, cyanColor lipgloss.Color
	successColor, warningColor, dangerColor                    lipgloss.Color
	panelBgColor, panelDimColor                                lipgloss.Color
	coolRamp, heatRamp                                         []colorStop
	titleStyle, headerStyle, mutedStyle                        lipgloss.Style
	successStyle, warningStyle, dangerStyle                    lipgloss.Style
	accentStyle, emptyBarStyle, panelBorderStyle               lipgloss.Style
	panelTitleStyle, panelTextStyle, panelShellStyle           lipgloss.Style
}

func captureThemeState() themeState {
	return themeState{
		inkColor: inkColor, mutedColor: mutedColor, violetColor: violetColor, magentaColor: magentaColor, cyanColor: cyanColor,
		successColor: successColor, warningColor: warningColor, dangerColor: dangerColor,
		panelBgColor: panelBgColor, panelDimColor: panelDimColor,
		coolRamp: cloneRamp(coolRamp), heatRamp: cloneRamp(heatRamp),
		titleStyle: titleStyle, headerStyle: headerStyle, mutedStyle: mutedStyle,
		successStyle: successStyle, warningStyle: warningStyle, dangerStyle: dangerStyle,
		accentStyle: accentStyle, emptyBarStyle: emptyBarStyle, panelBorderStyle: panelBorderStyle,
		panelTitleStyle: panelTitleStyle, panelTextStyle: panelTextStyle, panelShellStyle: panelShellStyle,
	}
}

func restoreThemeState(state themeState) {
	inkColor, mutedColor, violetColor, magentaColor, cyanColor = state.inkColor, state.mutedColor, state.violetColor, state.magentaColor, state.cyanColor
	successColor, warningColor, dangerColor = state.successColor, state.warningColor, state.dangerColor
	panelBgColor, panelDimColor = state.panelBgColor, state.panelDimColor
	coolRamp, heatRamp = cloneRamp(state.coolRamp), cloneRamp(state.heatRamp)
	titleStyle, headerStyle, mutedStyle = state.titleStyle, state.headerStyle, state.mutedStyle
	successStyle, warningStyle, dangerStyle = state.successStyle, state.warningStyle, state.dangerStyle
	accentStyle, emptyBarStyle, panelBorderStyle = state.accentStyle, state.emptyBarStyle, state.panelBorderStyle
	panelTitleStyle, panelTextStyle, panelShellStyle = state.panelTitleStyle, state.panelTextStyle, state.panelShellStyle
}

func applyTheme(theme Theme) {
	inkColor = theme.Ink
	mutedColor = theme.Muted
	violetColor = theme.Border
	magentaColor = theme.Title
	cyanColor = theme.Accent
	successColor = theme.Success
	warningColor = theme.Warning
	dangerColor = theme.Danger
	panelBgColor = theme.PanelBg
	panelDimColor = theme.PanelDim
	coolRamp = cloneRamp(theme.CoolRamp)
	heatRamp = cloneRamp(theme.HeatRamp)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).Background(theme.TitleBg).Padding(0, 1)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Border)
	mutedStyle = lipgloss.NewStyle().Foreground(theme.Muted)
	successStyle = lipgloss.NewStyle().Foreground(theme.Success).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(theme.Warning).Bold(true)
	dangerStyle = lipgloss.NewStyle().Foreground(theme.Danger).Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(theme.Accent).Bold(true)
	emptyBarStyle = lipgloss.NewStyle().Foreground(theme.PanelDim)
	panelBorderStyle = lipgloss.NewStyle().Foreground(theme.Border)
	panelTitleStyle = lipgloss.NewStyle().Foreground(theme.Title).Bold(true)
	panelTextStyle = lipgloss.NewStyle().Foreground(theme.Ink)
	panelShellStyle = lipgloss.NewStyle().Foreground(theme.Ink).Background(theme.PanelBg)
}

func cloneRamp(ramp []colorStop) []colorStop {
	out := make([]colorStop, len(ramp))
	copy(out, ramp)
	return out
}

func hexRed(hex string) float64 {
	return float64(parseHexByte(hex, 1))
}

func hexGreen(hex string) float64 {
	return float64(parseHexByte(hex, 3))
}

func hexBlue(hex string) float64 {
	return float64(parseHexByte(hex, 5))
}

func parseHexByte(hex string, start int) uint8 {
	if len(hex) < start+2 {
		return 0
	}
	value, err := strconv.ParseUint(hex[start:start+2], 16, 8)
	if err != nil {
		return 0
	}
	return uint8(value)
}
