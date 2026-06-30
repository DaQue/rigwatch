package ui

import (
	"fmt"
	"strings"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderUpdateNotification() string {
	if !m.updateInfo.Available {
		return ""
	}

	updateStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("226")).
		Bold(true)

	currentVer := m.updateInfo.CurrentVersion
	if !strings.HasPrefix(currentVer, "v") {
		currentVer = "v" + currentVer
	}

	return updateStyle.Render(fmt.Sprintf("\n\n⬆  Update available! %s → %s",
		currentVer, m.updateInfo.LatestVersion))
}

func (m Model) View() string {
	// Keep the render helpers' thresholds in sync with the live settings so
	// runtime changes (settings screen) take effect immediately.
	activeThresholds = m.settings.Thresholds

	if m.helpVisible {
		return m.renderHelp()
	}

	switch m.screen {
	case ScreenHostList:
		switch m.manageMode {
		case manageForm:
			return m.renderHostForm()
		case manageConfirmDelete:
			return m.renderDeleteConfirm()
		case manageHostKeyConfirm:
			return m.renderHostKeyConfirm()
		case managePassword:
			return m.renderPasswordPrompt()
		case manageBusy:
			return m.renderManageBusy()
		case manageResult:
			return m.renderManageResult()
		}

		listView := m.list.View()
		if len(m.failedHosts) > 0 {
			failedDetails := make([]string, 0, len(m.failedHosts))
			for hostName, err := range m.failedHosts {
				failedDetails = append(failedDetails, fmt.Sprintf("%s (%v)", hostName, err))
			}
			warning := fmt.Sprintf("\n⚠ Failed to connect: %s", strings.Join(failedDetails, ", "))
			listView += lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(warning)
		}
		if len(m.selectedHosts) > 0 {
			selectedNames := make([]string, len(m.selectedHosts))
			for i, h := range m.selectedHosts {
				selectedNames[i] = h.Name
			}
			footer := fmt.Sprintf("\nSelected (%d): %s", len(m.selectedHosts), strings.Join(selectedNames, ", "))
			listView += lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(footer)
		}
		manageHint := "\na add • e edit • d delete • i install key • o settings"
		listView += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(manageHint)
		versionFooter := fmt.Sprintf("\nv%s", internal.ShortVersion())
		listView += lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(versionFooter)
		listView += m.renderUpdateNotification()
		return listView

	case ScreenConnecting:
		return m.renderConnectingProgress()

	case ScreenDashboard:
		if len(m.selectedHosts) > 0 && m.currentHostIdx < len(m.selectedHosts) {
			currentHost := m.selectedHosts[m.currentHostIdx]

			if m.clients[currentHost.Name] == nil || m.sysInfos[currentHost.Name] == nil {
				return m.renderConnectingProgress()
			}

			hostIndicator := ""
			if len(m.selectedHosts) > 1 {
				hostIndicator = fmt.Sprintf(" [%d/%d]", m.currentHostIdx+1, len(m.selectedHosts))
			}
			return m.renderSingleHostTile(currentHost, hostIndicator) + m.renderUpdateNotification()
		}
		return m.renderConnectingProgress()

	case ScreenOverview:
		overviewView := m.renderOverview()
		return overviewView + m.renderUpdateNotification()

	case ScreenQuad:
		return m.renderQuad() + m.renderUpdateNotification()

	case ScreenSettings:
		return m.renderSettingsScreen()

	case ScreenPasswordPrompt:
		return m.renderConnectPasswordPrompt()
	}

	return ""
}
