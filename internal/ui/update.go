package ui

import (
	"fmt"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// hostListFooterReserve is the number of rows View() appends beneath the host
// list (selection summary, manage hint, version line). The list is sized to
// leave this much room so toggling a selection never overflows the screen.
const hostListFooterReserve = 3

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			for _, client := range m.clients {
				if client != nil {
					client.Close()
				}
			}
			return m, tea.Quit
		}

		// While a connection-manager sub-mode is active, all keys feed it.
		if m.screen == ScreenHostList && m.manageMode != manageNone {
			return m.updateManage(msg)
		}

		// The help overlay is modal: while open, ?/esc/q close it and any other
		// key is swallowed so it can't act on the screen underneath.
		if m.helpVisible {
			switch msg.String() {
			case "?", "esc", "q":
				m.helpVisible = false
			}
			return m, nil
		}

		// The settings screen owns all key input while it's open.
		if m.screen == ScreenSettings {
			return m.updateSettings(msg)
		}

		// The connect-time password prompt owns all key input while it's open.
		if m.screen == ScreenPasswordPrompt {
			return m.updatePasswordPrompt(msg)
		}

		// Host-list management shortcuts (suppressed while typing a filter).
		if m.screen == ScreenHostList && m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "a":
				m.startAddForm()
				return m, textinput.Blink
			case "e":
				if item, ok := m.list.SelectedItem().(hostItem); ok && !item.host.Local {
					m.startEditForm(item.host)
					return m, textinput.Blink
				}
				return m, nil
			case "d":
				if item, ok := m.list.SelectedItem().(hostItem); ok {
					switch {
					case item.host.Managed:
						m.pendingHost = item.host
						m.manageMode = manageConfirmDelete
					case !item.host.Local:
						m.manageMode = manageResult
						m.manageStatus = item.host.Name + " comes from ~/.ssh/config and isn't managed by rigwatch. Edit it there, or press e to save a managed copy."
						m.manageErr = true
					}
				}
				return m, nil
			case "i":
				if item, ok := m.list.SelectedItem().(hostItem); ok && !item.host.Local {
					m.pendingHost = item.host
					m.manageMode = manageBusy
					m.manageStatus = "Scanning host key for " + item.host.Name + "…"
					return m, scanHostKeyCmd(item.host)
				}
				return m, nil
			case "o":
				m.openSettings()
				return m, textinput.Blink
			}
		}

		switch msg.String() {
		case "?":
			// Open help, except while typing a host-list filter where "?" is a
			// literal character (let it fall through to the list).
			if m.screen == ScreenHostList && m.list.FilterState() == list.Filtering {
				break
			}
			m.helpVisible = true
			return m, nil
		case "q":
			for _, client := range m.clients {
				if client != nil {
					client.Close()
				}
			}
			return m, tea.Quit
		case " ":
			if m.screen == ScreenHostList {
				if item, ok := m.list.SelectedItem().(hostItem); ok {
					host := item.host
					found := false
					for i, h := range m.selectedHosts {
						if h.Name == host.Name {
							m.selectedHosts = append(m.selectedHosts[:i], m.selectedHosts[i+1:]...)
							found = true
							break
						}
					}
					if !found {
						m.selectedHosts = append(m.selectedHosts, host)
					}
					m.updateListSelection()
				}
				// Space is fully handled here; don't let it fall through to the
				// list (which would re-render and can shift the viewport).
				return m, nil
			}
		case "enter":
			if m.screen == ScreenHostList {
				if len(m.selectedHosts) == 0 {
					if item, ok := m.list.SelectedItem().(hostItem); ok {
						m.selectedHosts = append(m.selectedHosts, item.host)
					}
				}
				if len(m.selectedHosts) > 0 {
					// Collect passwords for any password-auth hosts first, then
					// connect (or connect immediately when none are needed).
					nm, cmd := m.startConnectFlow()
					return nm, cmd
				}
			}
		case "n":
			if m.screen == ScreenQuad {
				layout := paneLayout(m.width, m.height, len(m.selectedHosts), m.quadPage)
				if layout.Pages > 1 {
					m.quadPage = (m.quadPage + 1) % layout.Pages
					m.clampQuadFocus()
					m.quadStatus = ""
				}
			} else if m.screen == ScreenDashboard && len(m.selectedHosts) > 1 {
				m.currentHostIdx = (m.currentHostIdx + 1) % len(m.selectedHosts)
				nextHost := m.selectedHosts[m.currentHostIdx]
				if m.clients[nextHost.Name] == nil {
					return m, m.connectToHost(nextHost)
				}
			}
		case "p":
			if m.screen == ScreenQuad {
				layout := paneLayout(m.width, m.height, len(m.selectedHosts), m.quadPage)
				if layout.Pages > 1 {
					m.quadPage = (m.quadPage - 1 + layout.Pages) % layout.Pages
					m.clampQuadFocus()
					m.quadStatus = ""
				}
			}
		case "tab":
			if m.screen == ScreenQuad {
				m.moveQuadFocus(1)
			}
		case "shift+tab":
			if m.screen == ScreenQuad {
				m.moveQuadFocus(-1)
			}
		case "w":
			if m.screen == ScreenQuad {
				m.saveQuadLayout()
			}
		case "]":
			if m.screen == ScreenQuad {
				m.cycleFocusedHostTheme(1)
			}
		case "[":
			if m.screen == ScreenQuad {
				m.cycleFocusedHostTheme(-1)
			}
		case "g":
			switch m.screen {
			case ScreenDashboard, ScreenOverview:
				m.screen = ScreenQuad
				m.quadPage = 0
			case ScreenQuad:
				m.screen = ScreenDashboard
			}
		case "esc":
			if m.screen == ScreenQuad {
				m.screen = ScreenDashboard
			}
		case "c":
			if m.screen == ScreenDashboard || m.screen == ScreenOverview || m.screen == ScreenQuad {
				m.screen = ScreenHostList
				m.updateListSelection()
			}
		case "t":
			if m.screen == ScreenDashboard && len(m.selectedHosts) > 1 {
				m.screen = ScreenOverview
			} else if m.screen == ScreenOverview {
				m.screen = ScreenDashboard
			} else if m.screen == ScreenQuad {
				m.screen = ScreenDashboard
			}
		case "s":
			if m.screen == ScreenDashboard {
				if len(m.selectedHosts) > 0 {
					currentHost := m.selectedHosts[m.currentHostIdx]
					m.sshOnExit = currentHost.Name
					for _, client := range m.clients {
						if client != nil {
							client.Close()
						}
					}
					return m, tea.Quit
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve room for the footer block View() appends below the list
		// (selection summary + manage hint + version). Without this reserve,
		// selecting the first host adds the "Selected (N): …" line and pushes
		// the output one row past the screen, scrolling the whole view.
		m.list.SetSize(msg.Width, msg.Height-hostListFooterReserve)

	case ConnectedMsg:
		if msg.err != nil {
			m.failedHosts[msg.hostName] = msg.err
			// Drop a likely-bad password so re-selecting the host re-prompts.
			delete(m.passwords, msg.hostName)

			for i, h := range m.selectedHosts {
				if h.Name == msg.hostName {
					m.selectedHosts = append(m.selectedHosts[:i], m.selectedHosts[i+1:]...)
					break
				}
			}

			if len(m.selectedHosts) == 0 {
				m.screen = ScreenHostList
				m.updateListSelection()
			} else {
				if m.currentHostIdx >= len(m.selectedHosts) {
					m.currentHostIdx = len(m.selectedHosts) - 1
				}
			}
			return m, nil
		}
		m.clients[msg.hostName] = msg.client

		if m.screen == ScreenConnecting {
			return m, m.gatherSysInfoForHost(msg.hostName)
		}

		if m.screen == ScreenDashboard || m.screen == ScreenOverview || m.screen == ScreenQuad {
			return m, m.gatherSysInfoForHost(msg.hostName)
		}

	case SystemInfoMsg:
		if msg.err != nil {
			return m, nil
		}
		now := time.Now()
		if previous := m.sysInfos[msg.hostName]; previous != nil {
			elapsed := now.Sub(m.lastUpdates[msg.hostName]).Seconds()
			msg.info.Network = rateNetworkInterfaces(previous.Network, msg.info.Network, elapsed)
			msg.info.DiskIO = rateDiskIO(previous.DiskIO, msg.info.DiskIO, elapsed)
		}
		m.sysInfos[msg.hostName] = msg.info
		m.lastUpdates[msg.hostName] = now
		m.appendMetricHistory(msg.hostName, msg.info)

		if m.screen == ScreenConnecting && len(m.selectedHosts) > 0 {
			firstHost := m.selectedHosts[0]
			if m.clients[firstHost.Name] != nil && m.sysInfos[firstHost.Name] != nil {
				m.screen = m.postConnectScreen
				if m.screen == ScreenHostList || m.screen == ScreenConnecting {
					m.screen = ScreenDashboard
				}
				return m, m.tick()
			}
		}

	case UpdateCheckMsg:
		m.updateInfo = internal.UpdateInfo(msg)

	case TickMsg:
		// update every 10 seconds
		return m, tea.Batch(m.gatherAllSysInfo(), m.tick())

	case AnimationTickMsg:
		m.animationFrame++
		cmds := []tea.Cmd{animationTick()}
		// Start the spinner's self-tick only while it's actually on screen.
		if m.spinnerActive() && !m.spinnerRunning {
			m.spinnerRunning = true
			cmds = append(cmds, m.spinner.Tick)
		}
		return m, tea.Batch(cmds...)

	case hostSavedMsg:
		if msg.err != nil {
			m.manageStatus = msg.err.Error()
			m.manageErr = true
			m.manageMode = manageForm
			m.installAfterSave = false
			return m, nil
		}
		m.reloadHosts()
		if m.installAfterSave {
			m.installAfterSave = false
			host := m.pendingHost
			m.manageMode = manageBusy
			m.manageStatus = "Scanning host key for " + host.Name + "…"
			return m, scanHostKeyCmd(host)
		}
		m.resetManage()
		return m, nil

	case hostDeletedMsg:
		if msg.err != nil {
			m.manageMode = manageResult
			m.manageStatus = msg.err.Error()
			m.manageErr = true
			return m, nil
		}
		m.reloadHosts()
		m.resetManage()
		return m, nil

	case keyScanMsg:
		if msg.err != nil {
			m.manageMode = manageResult
			m.manageStatus = msg.err.Error()
			m.manageErr = true
			return m, nil
		}
		m.pendingHostKey = msg.key
		m.pendingFingerprint = msg.fingerprint
		m.manageMode = manageHostKeyConfirm
		return m, nil

	case testConnectedMsg:
		if m.manageMode != manageForm {
			return m, nil
		}
		if msg.err != nil {
			m.manageStatus = "✗ " + msg.err.Error()
			m.manageErr = true
		} else {
			m.manageStatus = "✓ Connection OK"
			m.manageErr = false
		}
		return m, nil

	case keyInstalledMsg:
		m.manageMode = manageResult
		if msg.err != nil {
			m.manageStatus = "Key installation failed: " + msg.err.Error()
			m.manageErr = true
		} else {
			m.manageStatus = "✓ Public key installed on " + m.pendingHost.Name
			m.manageErr = false
		}
		return m, nil
	}

	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	if !m.spinnerActive() {
		// Spinner is off screen: drop its self-rescheduling tick so we stop
		// repainting the whole UI while idle. The animation tick restarts it
		// when the spinner becomes visible again.
		spinnerCmd = nil
		m.spinnerRunning = false
	}

	if m.screen == ScreenHostList {
		switch m.manageMode {
		case manageForm:
			// The Auth row (focus == len(formInputs)) is a toggle, not a text
			// input, so there's no input to forward non-key messages to.
			if m.formFocus >= len(m.formInputs) {
				return m, spinnerCmd
			}
			var cmd tea.Cmd
			m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
			return m, tea.Batch(spinnerCmd, cmd)
		case managePassword:
			var cmd tea.Cmd
			m.passwordInput, cmd = m.passwordInput.Update(msg)
			return m, tea.Batch(spinnerCmd, cmd)
		case manageNone:
			var listCmd tea.Cmd
			m.list, listCmd = m.list.Update(msg)
			return m, tea.Batch(spinnerCmd, listCmd)
		}
		return m, spinnerCmd
	}

	return m, spinnerCmd
}

// spinnerActive reports whether the spinner is currently on screen. It is shown
// on the connecting screen, the manage "busy" screen, and the dashboard while
// the current host's telemetry hasn't arrived yet (which falls back to the
// connecting view). Everywhere else it's hidden, so its tick can be paused.
func (m Model) spinnerActive() bool {
	switch m.screen {
	case ScreenConnecting:
		return true
	case ScreenHostList:
		return m.manageMode == manageBusy
	case ScreenDashboard:
		if len(m.selectedHosts) > 0 && m.currentHostIdx < len(m.selectedHosts) {
			host := m.selectedHosts[m.currentHostIdx]
			return m.clients[host.Name] == nil || m.sysInfos[host.Name] == nil
		}
		return true
	}
	return false
}

func (m *Model) visibleQuadHostCount() int {
	layout := paneLayout(m.width, m.height, len(m.selectedHosts), m.quadPage)
	return max(0, layout.End-layout.Start)
}

func (m *Model) clampQuadFocus() {
	count := m.visibleQuadHostCount()
	if count == 0 {
		m.quadFocus = 0
		return
	}
	m.quadFocus = clampInt(m.quadFocus, 0, count-1)
}

func (m *Model) moveQuadFocus(delta int) {
	count := m.visibleQuadHostCount()
	if count == 0 {
		m.quadFocus = 0
		return
	}
	m.quadFocus = (m.quadFocus + delta + count) % count
}

func (m *Model) focusedQuadHost() (internal.SSHHost, bool) {
	layout := paneLayout(m.width, m.height, len(m.selectedHosts), m.quadPage)
	if layout.End <= layout.Start {
		return internal.SSHHost{}, false
	}
	m.clampQuadFocus()
	idx := layout.Start + m.quadFocus
	if idx < layout.Start || idx >= layout.End || idx >= len(m.selectedHosts) {
		return internal.SSHHost{}, false
	}
	return m.selectedHosts[idx], true
}

// saveQuadLayout persists the current grid (selected hosts, in order, plus the
// active page) so it can be restored on the next launch. Feedback is surfaced
// via quadStatus in the grid subtitle.
func (m *Model) saveQuadLayout() {
	if len(m.selectedHosts) == 0 {
		m.quadStatus = "nothing to save"
		return
	}
	names := make([]string, len(m.selectedHosts))
	for i, h := range m.selectedHosts {
		names[i] = h.Name
	}
	if err := SaveLayoutPreferences(LayoutPreferences{Hosts: names, Page: m.quadPage}); err != nil {
		m.quadStatus = "save failed: " + err.Error()
		return
	}
	m.quadStatus = fmt.Sprintf("✓ layout saved (%d hosts)", len(names))
}

func (m *Model) cycleFocusedHostTheme(delta int) {
	host, ok := m.focusedQuadHost()
	if !ok {
		return
	}
	current := m.themePrefs.ThemeNameForHost(host.Name)
	m.themePrefs.SetHostTheme(host.Name, nextThemeName(current, delta))
	_ = SaveThemePreferences(m.themePrefs)
}

func (m *Model) appendMetricHistory(hostName string, info *internal.SystemInfo) {
	if m.metricHistories == nil {
		m.metricHistories = make(map[string]metricHistory)
	}
	history := m.metricHistories[hostName]
	history.CPU = appendClampedSample(history.CPU, info.CPU.UsagePercent)
	history.RAM = appendClampedSample(history.RAM, info.RAM.UsagePercent)

	if len(info.GPUs) > 0 {
		var utilTotal, vramUsed, vramTotal int
		for _, gpu := range info.GPUs {
			utilTotal += gpu.Utilization
			vramUsed += gpu.VRAMUsed
			vramTotal += gpu.VRAMTotal
		}
		history.GPU = appendClampedSample(history.GPU, float64(utilTotal)/float64(len(info.GPUs)))
		if vramTotal > 0 {
			history.VRAM = appendClampedSample(history.VRAM, (float64(vramUsed)/float64(vramTotal))*100)
		}
	}

	// Track temperature: max across all sensors (includes CPU + GPU from merge)
	var maxTemp float64
	for _, t := range info.Temps {
		if t.Celsius > maxTemp {
			maxTemp = t.Celsius
		}
	}
	for _, gpu := range info.GPUs {
		if float64(gpu.Temperature) > maxTemp {
			maxTemp = float64(gpu.Temperature)
		}
	}
	history.Temp = appendClampedSample(history.Temp, maxTemp)

	// Track network: total throughput normalized to 0-100 (ceiling ~125 MB/s ≈ 1 Gbps)
	var totalRate float64
	for _, iface := range info.Network {
		totalRate += float64(iface.RXBps + iface.TXBps)
	}
	networkPercent := 0.0
	if totalRate > 0 {
		maxRate := 125.0 * 1024 * 1024 // 125 MB/s
		pct := totalRate / maxRate * 100
		if pct > 100 {
			pct = 100
		}
		networkPercent = pct
	}
	history.Network = appendClampedSample(history.Network, networkPercent)

	// Per-fan RPM history for the single-host fan trend bars.
	if len(info.Fans) > 0 {
		if history.Fans == nil {
			history.Fans = make(map[string][]float64, len(info.Fans))
		}
		for _, fan := range info.Fans {
			history.Fans[fan.Name] = appendClampedSample(history.Fans[fan.Name], float64(fan.RPM))
		}
	}

	m.metricHistories[hostName] = history
}

func appendClampedSample(samples []float64, sample float64) []float64 {
	samples = append(samples, sample)
	if len(samples) > metricHistoryLimit {
		return samples[len(samples)-metricHistoryLimit:]
	}
	return samples
}

func rateNetworkInterfaces(previous []internal.NetworkInfo, current []internal.NetworkInfo, elapsedSeconds float64) []internal.NetworkInfo {
	if elapsedSeconds <= 0 {
		return current
	}
	previousByName := make(map[string]internal.NetworkInfo, len(previous))
	for _, iface := range previous {
		previousByName[iface.Name] = iface
	}
	rated := make([]internal.NetworkInfo, len(current))
	for i, iface := range current {
		rated[i] = iface
		prev, ok := previousByName[iface.Name]
		if !ok || iface.RXBytes < prev.RXBytes || iface.TXBytes < prev.TXBytes {
			continue
		}
		rated[i].RXBps = uint64(float64(iface.RXBytes-prev.RXBytes) / elapsedSeconds)
		rated[i].TXBps = uint64(float64(iface.TXBytes-prev.TXBytes) / elapsedSeconds)
	}
	return rated
}

func rateDiskIO(previous []internal.DiskIOInfo, current []internal.DiskIOInfo, elapsedSeconds float64) []internal.DiskIOInfo {
	if elapsedSeconds <= 0 {
		return current
	}
	previousByDevice := make(map[string]internal.DiskIOInfo, len(previous))
	for _, dev := range previous {
		previousByDevice[dev.Device] = dev
	}
	rated := make([]internal.DiskIOInfo, len(current))
	for i, dev := range current {
		rated[i] = dev
		prev, ok := previousByDevice[dev.Device]
		if !ok || dev.ReadBytes < prev.ReadBytes || dev.WriteBytes < prev.WriteBytes {
			continue
		}
		rated[i].ReadBps = uint64(float64(dev.ReadBytes-prev.ReadBytes) / elapsedSeconds)
		rated[i].WriteBps = uint64(float64(dev.WriteBytes-prev.WriteBytes) / elapsedSeconds)
	}
	return rated
}
