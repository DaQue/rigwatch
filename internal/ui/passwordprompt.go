package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// passwordHostsNeeding returns the selected password-auth hosts that still need a
// password collected before they can be dialed (not already connected, no
// password captured this session).
func (m Model) passwordHostsNeeding() []string {
	var queue []string
	for _, host := range m.selectedHosts {
		if !host.PasswordAuth || host.Local {
			continue
		}
		if m.clients[host.Name] != nil {
			continue
		}
		if _, ok := m.passwords[host.Name]; ok {
			continue
		}
		queue = append(queue, host.Name)
	}
	return queue
}

// startConnectFlow either collects passwords first (password-auth hosts) or
// proceeds straight to connecting.
func (m Model) startConnectFlow() (Model, tea.Cmd) {
	queue := m.passwordHostsNeeding()
	if len(queue) > 0 {
		m.pwQueue = queue
		m.pwIndex = 0
		m.passwordInput = newPasswordInput()
		m.screen = ScreenPasswordPrompt
		return m, textinput.Blink
	}
	return m.beginConnect()
}

// beginConnect kicks off the actual SSH dials, landing on the connecting screen
// for a fresh session or the dashboard when adding hosts to a live one.
func (m Model) beginConnect() (Model, tea.Cmd) {
	m.failedHosts = make(map[string]error)
	if len(m.clients) > 0 {
		m.screen = ScreenDashboard
		if cmd := m.connectNewHosts(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}
	m.screen = ScreenConnecting
	return m, m.connectToHosts()
}

// updatePasswordPrompt drives the per-host password collection screen.
func (m Model) updatePasswordPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Abort the whole connect attempt and return to the picker.
		m.pwQueue = nil
		m.pwIndex = 0
		m.passwordInput.SetValue("")
		m.screen = ScreenHostList
		return m, nil
	case "enter":
		if m.pwIndex < len(m.pwQueue) {
			if m.passwords == nil {
				m.passwords = make(map[string]string)
			}
			m.passwords[m.pwQueue[m.pwIndex]] = m.passwordInput.Value()
			m.passwordInput.SetValue("")
			m.pwIndex++
		}
		if m.pwIndex >= len(m.pwQueue) {
			nm, cmd := m.beginConnect()
			return nm, cmd
		}
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.passwordInput, cmd = m.passwordInput.Update(msg)
	return m, cmd
}

func (m Model) renderConnectPasswordPrompt() string {
	width := managePanelWidth(m.width)

	hostName := ""
	if m.pwIndex < len(m.pwQueue) {
		hostName = m.pwQueue[m.pwIndex]
	}
	user := ""
	for _, h := range m.selectedHosts {
		if h.Name == hostName {
			user = h.User
			break
		}
	}
	if user == "" {
		user = "(default user)"
	}

	var b strings.Builder
	if len(m.pwQueue) > 1 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Host %d of %d", m.pwIndex+1, len(m.pwQueue))))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf("Password for %s\n", accentStyle.Render(hostName)))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Authenticating as %s", user)))
	b.WriteString("\n\n")
	b.WriteString("Password  " + m.passwordInput.View())
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("enter continue • esc cancel"))
	return renderPanel("PASSWORD REQUIRED", b.String(), width)
}
