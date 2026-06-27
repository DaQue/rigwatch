package ui

import (
	"time"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) connectToHosts() tea.Cmd {
	var cmds []tea.Cmd
	for _, host := range m.selectedHosts {
		h := host
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
	return func() tea.Msg {
		client, err := internal.NewSSHClient(host)
		return ConnectedMsg{hostName: host.Name, client: client, err: err}
	}
}

func (m Model) connectNewHosts() tea.Cmd {
	var cmds []tea.Cmd
	for _, host := range m.selectedHosts {
		if m.clients[host.Name] == nil {
			h := host
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

func animationTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return AnimationTickMsg(t)
	})
}
