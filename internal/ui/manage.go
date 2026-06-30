package ui

import (
	"strings"

	"github.com/allisonhere/rigwatch/internal"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/crypto/ssh"
)

var formFieldLabels = []string{"Name", "HostName", "User", "Port", "IdentityFile"}

type hostSavedMsg struct{ err error }
type hostDeletedMsg struct{ err error }
type keyScanMsg struct {
	fingerprint string
	key         ssh.PublicKey
	err         error
}
type keyInstalledMsg struct{ err error }
type testConnectedMsg struct{ err error }

func newFormInput(placeholder, value string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.CharLimit = 128
	ti.Width = 40
	ti.Prompt = ""
	return ti
}

func blankFormInputs() []textinput.Model {
	return []textinput.Model{
		newFormInput("myserver", ""),
		newFormInput("10.0.0.5 or host.example.com", ""),
		newFormInput("ubuntu", ""),
		newFormInput("22", ""),
		newFormInput("~/.ssh/id_ed25519 (optional)", ""),
	}
}

func (m *Model) startAddForm() {
	m.formInputs = blankFormInputs()
	m.formOriginalName = ""
	m.formAuthPassword = false
	m.manageStatus = ""
	m.manageErr = false
	m.manageMode = manageForm
	m.setFormFocus(0)
}

func (m *Model) startEditForm(host internal.SSHHost) {
	m.formInputs = []textinput.Model{
		newFormInput("myserver", host.Name),
		newFormInput("10.0.0.5 or host.example.com", host.Hostname),
		newFormInput("ubuntu", host.User),
		newFormInput("22", host.Port),
		newFormInput("~/.ssh/id_ed25519 (optional)", host.IdentityFile),
	}
	m.formOriginalName = host.Name
	m.formAuthPassword = host.PasswordAuth
	m.manageStatus = ""
	m.manageErr = false
	m.manageMode = manageForm
	m.setFormFocus(0)
}

// formAuthRow is the focus index of the Auth toggle: it sits just past the text
// inputs, so the form has len(formInputs)+1 focus stops.
func (m Model) formAuthRow() int { return len(m.formInputs) }

func (m *Model) setFormFocus(i int) {
	last := m.formAuthRow()
	if i < 0 {
		i = last
	}
	if i > last {
		i = 0
	}
	for j := range m.formInputs {
		// The Auth row (i == last) focuses no text input.
		if j == i {
			m.formInputs[j].Focus()
		} else {
			m.formInputs[j].Blur()
		}
	}
	m.formFocus = i
}

func (m Model) buildHostFromForm() internal.SSHHost {
	return internal.SSHHost{
		Name:         strings.TrimSpace(m.formInputs[0].Value()),
		Hostname:     strings.TrimSpace(m.formInputs[1].Value()),
		User:         strings.TrimSpace(m.formInputs[2].Value()),
		Port:         strings.TrimSpace(m.formInputs[3].Value()),
		IdentityFile: strings.TrimSpace(m.formInputs[4].Value()),
		PasswordAuth: m.formAuthPassword,
		Managed:      true,
	}
}

func (m *Model) resetManage() {
	m.manageMode = manageNone
	m.manageStatus = ""
	m.manageErr = false
	m.pendingHost = internal.SSHHost{}
	m.pendingHostKey = nil
	m.pendingFingerprint = ""
	m.installAfterSave = false
	m.passwordInput.SetValue("")
}

func (m *Model) reloadHosts() {
	hosts, err := internal.LoadAllHosts()
	if err != nil {
		return
	}
	m.hosts = hosts

	selectedMap := make(map[string]bool)
	for _, h := range m.selectedHosts {
		selectedMap[h.Name] = true
	}
	items := make([]list.Item, len(hosts))
	for i, h := range hosts {
		items[i] = hostItem{host: h, selected: selectedMap[h.Name]}
	}
	m.list.SetItems(items)
}

// updateManage handles key input while a connection-manager sub-mode is active.
func (m Model) updateManage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.manageMode {
	case manageForm:
		switch msg.String() {
		case "esc":
			m.resetManage()
			return m, nil
		case "enter", "ctrl+k":
			host := m.buildHostFromForm()
			if err := internal.ValidateHost(host); err != nil {
				m.manageStatus = err.Error()
				m.manageErr = true
				return m, nil
			}
			m.manageStatus = ""
			m.manageErr = false
			m.pendingHost = host
			m.installAfterSave = msg.String() == "ctrl+k"
			return m, saveHostCmd(host, m.formOriginalName)
		case "ctrl+t":
			host := m.buildHostFromForm()
			if err := internal.ValidateHost(host); err != nil {
				m.manageStatus = err.Error()
				m.manageErr = true
				return m, nil
			}
			m.manageStatus = "Testing connection to " + host.Name + "…"
			m.manageErr = false
			return m, testConnectionCmd(host)
		case "tab", "down":
			m.setFormFocus(m.formFocus + 1)
			return m, textinput.Blink
		case "shift+tab", "up":
			m.setFormFocus(m.formFocus - 1)
			return m, textinput.Blink
		}
		// The Auth row is a toggle, not a text field: left/right/space flip it and
		// all other keys are ignored so they don't leak into a (blurred) input.
		if m.formFocus == m.formAuthRow() {
			switch msg.String() {
			case "left", "right", " ":
				m.formAuthPassword = !m.formAuthPassword
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.formInputs[m.formFocus], cmd = m.formInputs[m.formFocus].Update(msg)
		return m, cmd

	case manageConfirmDelete:
		switch msg.String() {
		case "y", "Y":
			name := m.pendingHost.Name
			m.manageMode = manageBusy
			m.manageStatus = "Deleting " + name + "…"
			return m, deleteHostCmd(name)
		case "n", "N", "esc":
			m.resetManage()
		}
		return m, nil

	case manageHostKeyConfirm:
		switch msg.String() {
		case "y", "Y":
			hostname := m.pendingHost.Hostname
			if hostname == "" {
				hostname = m.pendingHost.Name
			}
			port := m.pendingHost.Port
			if port == "" {
				port = "22"
			}
			if err := internal.AddKnownHost(hostname, port, m.pendingHostKey); err != nil {
				m.manageMode = manageResult
				m.manageStatus = "Failed to trust host key: " + err.Error()
				m.manageErr = true
				return m, nil
			}
			m.passwordInput = newPasswordInput()
			m.manageMode = managePassword
			return m, textinput.Blink
		case "n", "N", "esc":
			m.resetManage()
		}
		return m, nil

	case managePassword:
		switch msg.String() {
		case "esc":
			m.resetManage()
			return m, nil
		case "enter":
			password := m.passwordInput.Value()
			m.passwordInput.SetValue("")
			host := m.pendingHost
			m.manageMode = manageBusy
			m.manageStatus = "Installing key on " + host.Name + "…"
			return m, installKeyCmd(host, password)
		}
		var cmd tea.Cmd
		m.passwordInput, cmd = m.passwordInput.Update(msg)
		return m, cmd

	case manageResult:
		m.resetManage()
		return m, nil
	}

	// manageBusy: ignore input until the in-flight command reports back.
	return m, nil
}

func newPasswordInput() textinput.Model {
	pi := textinput.New()
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.CharLimit = 256
	pi.Width = 40
	pi.Prompt = ""
	pi.Focus()
	return pi
}

func saveHostCmd(host internal.SSHHost, oldName string) tea.Cmd {
	return func() tea.Msg {
		if oldName != "" && oldName != host.Name {
			if err := internal.DeleteManagedHost(oldName); err != nil {
				return hostSavedMsg{err: err}
			}
		}
		return hostSavedMsg{err: internal.SaveManagedHost(host)}
	}
}

func deleteHostCmd(name string) tea.Cmd {
	return func() tea.Msg {
		return hostDeletedMsg{err: internal.DeleteManagedHost(name)}
	}
}

func scanHostKeyCmd(host internal.SSHHost) tea.Cmd {
	return func() tea.Msg {
		key, fingerprint, err := internal.ScanHostKey(host)
		return keyScanMsg{fingerprint: fingerprint, key: key, err: err}
	}
}

func installKeyCmd(host internal.SSHHost, password string) tea.Cmd {
	return func() tea.Msg {
		pub, err := internal.EnsureLocalKeyPair()
		if err != nil {
			return keyInstalledMsg{err: err}
		}
		return keyInstalledMsg{err: internal.InstallPublicKey(host, password, pub)}
	}
}

func testConnectionCmd(host internal.SSHHost) tea.Cmd {
	return func() tea.Msg {
		client, err := internal.NewSSHClient(host)
		if client != nil {
			client.Close()
		}
		return testConnectedMsg{err: err}
	}
}
