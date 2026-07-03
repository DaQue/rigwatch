package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

func TestModeMenuSwallowsGlobalQuitKey(t *testing.T) {
	m := InitialModel(nil, time.Second)
	m.screen = ScreenDashboard
	(&m).openModeMenu()

	updated, cmd := m.Update(key("q"))
	m = updated.(Model)
	if !m.modeMenuOpen {
		t.Fatalf("mode menu should remain open after an unrelated key")
	}
	if m.screen != ScreenDashboard {
		t.Fatalf("screen changed under mode menu: %v", m.screen)
	}
	if cmd != nil {
		t.Fatalf("mode menu should swallow q without returning a command")
	}
}

func TestSettingsScreenOwnsGlobalQuitKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	m := InitialModel(nil, time.Second)
	m.screen = ScreenHostList
	(&m).openSettings()

	updated, cmd := m.Update(key("q"))
	m = updated.(Model)
	if m.screen != ScreenSettings {
		t.Fatalf("settings should keep focus after q, got screen %v", m.screen)
	}
	if m.settingsForm == nil {
		t.Fatalf("settings form should remain open")
	}
	if cmd != nil {
		t.Fatalf("settings theme row should handle q without returning a command")
	}
}

func TestConnectingTelemetryLandsOnRequestedScreen(t *testing.T) {
	hosts := testHosts("alpha")
	m := InitialModelWithHosts(hosts, hosts, time.Second)
	m.screen = ScreenConnecting
	m.postConnectScreen = ScreenQuad
	m.clients["alpha"] = &internal.SSHClient{}

	updated, cmd := m.Update(SystemInfoMsg{
		hostName: "alpha",
		info:     &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 42}},
	})
	m = updated.(Model)
	if m.screen != ScreenQuad {
		t.Fatalf("screen after first telemetry = %v, want ScreenQuad", m.screen)
	}
	if m.sysInfos["alpha"] == nil {
		t.Fatalf("telemetry was not stored")
	}
	if cmd == nil {
		t.Fatalf("expected telemetry handoff to schedule metric tick")
	}
}
