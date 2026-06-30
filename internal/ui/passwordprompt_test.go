package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

func TestPasswordHostsNeeding(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.selectedHosts = []internal.SSHHost{
		{Name: "keybox"},                                   // key auth — never prompts
		{Name: "pw1", PasswordAuth: true},                  // needs a password
		{Name: "pw2", PasswordAuth: true},                  // already has one
		{Name: "pwlocal", PasswordAuth: true, Local: true}, // local — no prompt
	}
	m.passwords["pw2"] = "secret"

	queue := m.passwordHostsNeeding()
	if len(queue) != 1 || queue[0] != "pw1" {
		t.Fatalf("passwordHostsNeeding = %v, want [pw1]", queue)
	}
}

func TestStartConnectFlowPromptsForPasswordHosts(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.selectedHosts = []internal.SSHHost{{Name: "pw1", PasswordAuth: true}}

	nm, _ := m.startConnectFlow()
	if nm.screen != ScreenPasswordPrompt {
		t.Fatalf("screen = %v, want ScreenPasswordPrompt", nm.screen)
	}
	if len(nm.pwQueue) != 1 || nm.pwQueue[0] != "pw1" {
		t.Fatalf("pwQueue = %v", nm.pwQueue)
	}
}

func TestStartConnectFlowSkipsPromptForKeyHosts(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.selectedHosts = []internal.SSHHost{{Name: "keybox"}}

	nm, cmd := m.startConnectFlow()
	if nm.screen == ScreenPasswordPrompt {
		t.Fatalf("key-only hosts should not open the password prompt")
	}
	if nm.screen != ScreenConnecting {
		t.Fatalf("screen = %v, want ScreenConnecting", nm.screen)
	}
	if cmd == nil {
		t.Fatalf("expected a connect command")
	}
}

func TestPasswordPromptCollectsThenConnects(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.selectedHosts = []internal.SSHHost{
		{Name: "pw1", PasswordAuth: true},
		{Name: "pw2", PasswordAuth: true},
	}

	nm, _ := m.startConnectFlow()
	m = nm

	// First host's password.
	m.passwordInput.SetValue("first")
	upd, _ := m.updatePasswordPrompt(key("enter"))
	m = upd.(Model)
	if m.screen != ScreenPasswordPrompt {
		t.Fatalf("expected to stay on prompt for the second host, got %v", m.screen)
	}
	if m.passwords["pw1"] != "first" {
		t.Fatalf("pw1 password = %q, want 'first'", m.passwords["pw1"])
	}

	// Second host's password completes the queue and begins connecting.
	m.passwordInput.SetValue("second")
	upd, cmd := m.updatePasswordPrompt(key("enter"))
	m = upd.(Model)
	if m.passwords["pw2"] != "second" {
		t.Fatalf("pw2 password = %q, want 'second'", m.passwords["pw2"])
	}
	if m.screen != ScreenConnecting {
		t.Fatalf("screen = %v, want ScreenConnecting after collecting all", m.screen)
	}
	if cmd == nil {
		t.Fatalf("expected a connect command after collecting passwords")
	}
}

func TestPasswordPromptEscCancels(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.selectedHosts = []internal.SSHHost{{Name: "pw1", PasswordAuth: true}}
	nm, _ := m.startConnectFlow()
	m = nm

	upd, _ := m.updatePasswordPrompt(key("esc"))
	m = upd.(Model)
	if m.screen != ScreenHostList {
		t.Fatalf("esc should return to host list, got %v", m.screen)
	}
	if len(m.pwQueue) != 0 {
		t.Fatalf("queue should be cleared on cancel, got %v", m.pwQueue)
	}
}

func TestFormBuildsPasswordAuthHost(t *testing.T) {
	m := InitialModel(nil, time.Second)
	(&m).startEditForm(internal.SSHHost{Name: "box", Hostname: "h", User: "u", Port: "22", PasswordAuth: true})
	if !m.formAuthPassword {
		t.Fatalf("startEditForm should load PasswordAuth into the toggle")
	}
	if !m.buildHostFromForm().PasswordAuth {
		t.Fatalf("buildHostFromForm should carry PasswordAuth")
	}
}
