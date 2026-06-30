package ui

import (
	"time"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
)

// withPassword fills in the session-collected password for a password-auth host
// just before dialing, so the credential lives only on the dial goroutine.
func (m Model) withPassword(host internal.SSHHost) internal.SSHHost {
	if host.PasswordAuth {
		host.Password = m.passwords[host.Name]
	}
	return host
}

func (m Model) connectToHosts() tea.Cmd {
	var cmds []tea.Cmd
	for _, host := range m.selectedHosts {
		h := m.withPassword(host)
		cmds = append(cmds, func() tea.Msg {
			client, err := internal.NewSSHClient(h)
			return ConnectedMsg{hostName: h.Name, client: client, err: err}
		})
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

func (m Model) connectToHost(host internal.SSHHost) tea.Cmd {
	h := m.withPassword(host)
	return func() tea.Msg {
		client, err := internal.NewSSHClient(h)
		return ConnectedMsg{hostName: h.Name, client: client, err: err}
	}
}

func (m Model) connectNewHosts() tea.Cmd {
	var cmds []tea.Cmd
	for _, host := range m.selectedHosts {
		if m.clients[host.Name] == nil {
			h := m.withPassword(host)
			cmds = append(cmds, func() tea.Msg {
				client, err := internal.NewSSHClient(h)
				return ConnectedMsg{hostName: h.Name, client: client, err: err}
			})
		}
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

func (m Model) gatherAllSysInfo() tea.Cmd {
	var cmds []tea.Cmd
	for _, host := range m.selectedHosts {
		h := host
		client := m.clients[h.Name]
		if client != nil {
			cmds = append(cmds, func() tea.Msg {
				info, err := internal.GatherSystemInfo(client)
				return SystemInfoMsg{hostName: h.Name, info: info, err: err}
			})
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) gatherSysInfoForHost(hostName string) tea.Cmd {
	client := m.clients[hostName]
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		info, err := internal.GatherSystemInfo(client)
		return SystemInfoMsg{hostName: hostName, info: info, err: err}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(m.updateInterval, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// animationInterval throttles the cosmetic signal-bar animation. Every tick
// triggers a full re-render, so this is the dominant idle-CPU lever; 250ms
// (4fps) keeps the shimmer visible while cutting repaints ~2.5x versus 100ms.
const animationInterval = 250 * time.Millisecond

func animationTick() tea.Cmd {
	return tea.Tick(animationInterval, func(t time.Time) tea.Msg {
		return AnimationTickMsg(t)
	})
}
