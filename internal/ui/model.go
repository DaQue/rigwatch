package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/ssh"
)

type Screen int

const (
	ScreenHostList Screen = iota
	ScreenConnecting
	ScreenDashboard
	ScreenOverview
	ScreenQuad
	ScreenSettings
	ScreenPasswordPrompt
)

type Model struct {
	screen            Screen
	hosts             []internal.SSHHost
	selectedHosts     []internal.SSHHost
	currentHostIdx    int
	list              list.Model
	spinner           spinner.Model
	spinnerRunning    bool
	clients           map[string]*internal.SSHClient
	sysInfos          map[string]*internal.SystemInfo
	lastUpdates       map[string]time.Time
	updateInterval    time.Duration
	failedHosts       map[string]error
	width             int
	height            int
	sshOnExit         string
	updateInfo        internal.UpdateInfo
	animationFrame    int
	metricHistories   map[string]metricHistory
	quadPage          int
	quadFocus         int
	headerFocus       int    // grid header toolbar focus; -1 = a pane is focused
	focusFramesLeft   int    // animation frames the grid focus highlight stays visible
	gridTilesPerPage  int    // tiles per page on the grid screen: 2 (dual) or 4 (quad)
	quadStatus        string // transient feedback for the quad view (e.g. "layout saved")
	postConnectScreen Screen // screen to land on once the connecting screen finishes
	themePrefs        ThemePreferences
	settings          Settings
	settingsForm      *settingsFormState
	helpVisible       bool
	modeMenuOpen      bool // display-mode picker overlay
	modeMenuIdx       int

	// Connection-manager sub-state (host-list screen only).
	manageMode         manageMode
	formInputs         []textinput.Model
	formFocus          int
	formOriginalName   string
	passwordInput      textinput.Model
	manageStatus       string
	manageErr          bool
	pendingHost        internal.SSHHost
	pendingHostKey     ssh.PublicKey
	pendingFingerprint string
	installAfterSave   bool
	formAuthPassword   bool // edit-form Auth toggle: password vs key

	// Connect-time password collection (password-auth hosts only).
	passwords map[string]string // hostName -> password, session-only, never persisted
	pwQueue   []string          // hosts still needing a password before connecting
	pwIndex   int               // position within pwQueue
}

type manageMode int

const (
	manageNone manageMode = iota
	manageForm
	manageConfirmDelete
	manageHostKeyConfirm
	managePassword
	manageBusy
	manageResult
)

// metricHistoryLimit is how many samples each trend keeps. Braille sparklines
// pack two samples per cell, so this is sized to fill the widest trend the grid
// renders (56 cells) without resampling padding.
const metricHistoryLimit = 112

// focusHoldFrames is how many animation ticks (~250ms each) the grid focus
// highlight stays lit after the user moves focus, before it fades out so it
// isn't permanently left on a pane.
const focusHoldFrames = 6

type metricHistory struct {
	CPU     []float64
	GPU     []float64
	VRAM    []float64
	RAM     []float64
	Temp    []float64
	Network []float64
	Fans    map[string][]float64 // RPM history keyed by fan name
}

type TickMsg time.Time

type AnimationTickMsg time.Time

type UpdateCheckMsg internal.UpdateInfo
type SystemInfoMsg struct {
	hostName string
	info     *internal.SystemInfo
	err      error
}

type ConnectedMsg struct {
	hostName string
	client   *internal.SSHClient
	err      error
}

type hostItem struct {
	host         internal.SSHHost
	selected     bool
	severity     Severity
	hasTelemetry bool
}

func (h hostItem) FilterValue() string { return h.host.Name }
func (h hostItem) Title() string {
	prefix := "  "
	if h.selected {
		prefix = "✓ "
	}
	// Show a health dot only once telemetry exists, so the picker doesn't imply
	// "healthy" for hosts that haven't been sampled yet.
	if h.hasTelemetry {
		prefix += severityGlyph(h.severity) + " "
	}
	return prefix + h.host.Name
}
func (h hostItem) Description() string {
	if h.host.Local {
		return "  local machine"
	}
	if h.host.Hostname != "" {
		return fmt.Sprintf("  %s@%s:%s", h.host.User, censorHostname(h.host.Hostname), h.host.Port)
	}
	return ""
}

func censorHostname(hostname string) string {
	if hostname == "" {
		return ""
	}

	if strings.Contains(hostname, ".") {
		parts := strings.Split(hostname, ".")
		if len(parts) >= 4 {
			lastOctet := parts[len(parts)-1]
			lastPart := lastOctet
			if len(lastOctet) > 2 {
				lastPart = lastOctet[len(lastOctet)-2:]
			}
			return fmt.Sprintf("%s.***.***%s", parts[0], lastPart)
		}
	}

	if len(hostname) <= 8 {
		if len(hostname) <= 3 {
			return hostname
		}
		return hostname[:2] + strings.Repeat("*", len(hostname)-2)
	}

	return hostname[:3] + strings.Repeat("*", 5) + hostname[len(hostname)-3:]
}

func formatInterval(interval time.Duration) string {
	seconds := interval.Seconds()
	if seconds < 1 {
		return fmt.Sprintf("%.2fs", seconds)
	} else if seconds < 10 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	return fmt.Sprintf("%.0fs", seconds)
}

func InitialModel(hosts []internal.SSHHost, updateInterval time.Duration) Model {
	items := make([]list.Item, len(hosts))
	for i, h := range hosts {
		items[i] = hostItem{host: h, selected: false}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select SSH Hosts to Monitor (Space to select, Enter to confirm)"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		screen:            ScreenHostList,
		postConnectScreen: ScreenDashboard,
		gridTilesPerPage:  quadPageSize,
		headerFocus:       -1,
		hosts:             hosts,
		list:              l,
		spinner:           s,
		clients:           make(map[string]*internal.SSHClient),
		sysInfos:          make(map[string]*internal.SystemInfo),
		lastUpdates:       make(map[string]time.Time),
		failedHosts:       make(map[string]error),
		passwords:         make(map[string]string),
		metricHistories:   make(map[string]metricHistory),
		updateInterval:    updateInterval,
		themePrefs:        loadThemePreferencesOrDefault(),
		settings:          loadSettingsOrDefault(),
	}
}

func InitialModelWithHost(host internal.SSHHost, updateInterval time.Duration) Model {
	return InitialModelWithHosts([]internal.SSHHost{host}, []internal.SSHHost{host}, updateInterval)
}

func InitialModelWithHosts(allHosts []internal.SSHHost, selectedHosts []internal.SSHHost, updateInterval time.Duration) Model {
	items := make([]list.Item, len(allHosts))
	selectedMap := make(map[string]bool)
	for _, h := range selectedHosts {
		selectedMap[h.Name] = true
	}

	for i, h := range allHosts {
		items[i] = hostItem{host: h, selected: selectedMap[h.Name]}
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "Select SSH Hosts to Monitor (Space to select, Enter to confirm)"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		screen:            ScreenConnecting,
		postConnectScreen: ScreenDashboard,
		gridTilesPerPage:  quadPageSize,
		headerFocus:       -1,
		hosts:             allHosts,
		selectedHosts:     selectedHosts,
		currentHostIdx:    0,
		list:              l,
		spinner:           s,
		clients:           make(map[string]*internal.SSHClient),
		sysInfos:          make(map[string]*internal.SystemInfo),
		lastUpdates:       make(map[string]time.Time),
		failedHosts:       make(map[string]error),
		passwords:         make(map[string]string),
		metricHistories:   make(map[string]metricHistory),
		updateInterval:    updateInterval,
		themePrefs:        loadThemePreferencesOrDefault(),
		settings:          loadSettingsOrDefault(),
	}
}

func (m Model) GetSSHOnExit() string {
	return m.sshOnExit
}

func (m *Model) updateListSelection() {
	items := m.list.Items()
	selectedMap := make(map[string]bool)
	for _, h := range m.selectedHosts {
		selectedMap[h.Name] = true
	}

	newItems := make([]list.Item, len(items))
	for i, item := range items {
		if hi, ok := item.(hostItem); ok {
			hi.selected = selectedMap[hi.host.Name]
			if info := m.sysInfos[hi.host.Name]; info != nil {
				hi.hasTelemetry = true
				hi.severity = m.worstHostSeverity(hi.host.Name)
			} else {
				hi.hasTelemetry = false
				hi.severity = SevOK
			}
			newItems[i] = hi
		}
	}
	m.list.SetItems(newItems)
}

func (m Model) Init() tea.Cmd {
	// The spinner tick is started on demand by the animation tick (only while the
	// spinner is actually visible), so it isn't kicked off here.
	if m.screen == ScreenConnecting && len(m.selectedHosts) > 0 {
		return tea.Batch(animationTick(), m.connectToHosts(), checkForUpdates(m.settings))
	}
	return tea.Batch(animationTick(), checkForUpdates(m.settings))
}

func loadThemePreferencesOrDefault() ThemePreferences {
	prefs, err := LoadThemePreferences()
	if err != nil {
		return ThemePreferences{Hosts: make(map[string]string)}
	}
	return prefs
}

func loadSettingsOrDefault() Settings {
	settings, err := LoadSettings()
	if err != nil {
		return DefaultSettings()
	}
	return settings
}
