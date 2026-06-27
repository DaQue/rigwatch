package ui

import (
	"fmt"
	"strings"
)

func managePanelWidth(width int) int {
	return clampInt(width-4, 44, 72)
}

func (m Model) renderHostForm() string {
	width := managePanelWidth(m.width)
	title := "ADD HOST"
	if m.formOriginalName != "" {
		title = "EDIT HOST"
	}

	var b strings.Builder
	for i, in := range m.formInputs {
		pointer := "  "
		if i == m.formFocus {
			pointer = accentStyle.Render("▸ ")
		}
		b.WriteString(fmt.Sprintf("%s%-13s %s\n", pointer, formFieldLabels[i], in.View()))
	}
	b.WriteString("\n")
	if m.manageStatus != "" {
		if m.manageErr {
			b.WriteString(dangerStyle.Render(m.manageStatus))
		} else {
			b.WriteString(accentStyle.Render(m.manageStatus))
		}
		b.WriteString("\n")
	}
	b.WriteString(mutedStyle.Render("tab/↑↓ move • enter save • ctrl+t test • ctrl+k save & install key • esc cancel"))
	return renderPanel(title, b.String(), width)
}

func (m Model) renderDeleteConfirm() string {
	width := managePanelWidth(m.width)
	body := fmt.Sprintf("Delete managed host %s?\n\n%s",
		accentStyle.Render(m.pendingHost.Name),
		mutedStyle.Render("y confirm • n cancel"))
	return renderPanel("DELETE HOST", body, width)
}

func (m Model) renderHostKeyConfirm() string {
	width := managePanelWidth(m.width)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Host %s presented this key:\n\n", accentStyle.Render(m.pendingHost.Name)))
	b.WriteString(panelTextStyle.Render(m.pendingFingerprint))
	b.WriteString("\n\n")
	b.WriteString(warningStyle.Render("Trust this host and add it to known_hosts?"))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render("y trust & continue • n cancel"))
	return renderPanel("VERIFY HOST KEY", b.String(), width)
}

func (m Model) renderPasswordPrompt() string {
	width := managePanelWidth(m.width)
	user := m.pendingHost.User
	if user == "" {
		user = "(default user)"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Install your public key on %s\n", accentStyle.Render(m.pendingHost.Name)))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Authenticating as %s", user)))
	b.WriteString("\n\n")
	b.WriteString("Password  " + m.passwordInput.View())
	b.WriteString("\n\n")
	b.WriteString(mutedStyle.Render("enter install • esc cancel"))
	return renderPanel("INSTALL SSH KEY", b.String(), width)
}

func (m Model) renderManageBusy() string {
	width := managePanelWidth(m.width)
	body := m.spinner.View() + " " + panelTextStyle.Render(m.manageStatus)
	return renderPanel("WORKING", body, width)
}

func (m Model) renderManageResult() string {
	width := managePanelWidth(m.width)
	style := successStyle
	if m.manageErr {
		style = dangerStyle
	}
	body := style.Render(m.manageStatus) + "\n\n" + mutedStyle.Render("press any key to continue")
	return renderPanel("RESULT", body, width)
}
