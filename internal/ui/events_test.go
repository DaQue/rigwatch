package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/allisonhere/rigwatch/internal"
)

func eventModel() Model {
	m := InitialModel(nil, 5*time.Second)
	m.settings = DefaultSettings()
	m.events = openEventLog("")
	return m
}

func texts(m Model) []string {
	var out []string
	for _, ev := range m.events.events {
		out = append(out, ev.Kind+":"+ev.Text)
	}
	return out
}

func cpuInfo(pct float64) *internal.SystemInfo {
	return &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: pct}}
}

func TestRecordEventsLogsAlertLifecycle(t *testing.T) {
	m := eventModel()
	now := t0
	m.recordEvents("rig", cpuInfo(10), nil, now) // first poll: start marker only
	if got := texts(m); len(got) != 1 || got[0] != "info:monitoring started" {
		t.Fatalf("first poll events = %v", got)
	}

	m.recordEvents("rig", cpuInfo(90), nil, now.Add(10*time.Second)) // warn (85)
	m.recordEvents("rig", cpuInfo(91), nil, now.Add(15*time.Second)) // still warn: no new event
	m.recordEvents("rig", cpuInfo(97), nil, now.Add(20*time.Second)) // crit (95): escalation logs
	m.recordEvents("rig", cpuInfo(20), nil, now.Add(5*time.Minute))  // recovered
	got := strings.Join(texts(m), "|")
	want := "info:monitoring started|alert:CPU at 90% — warning|alert:CPU at 97% — critical|clear:CPU back to normal"
	if got != want {
		t.Fatalf("events =\n %s\nwant\n %s", got, want)
	}
	if m.events.events[2].Sev != SevCrit {
		t.Errorf("escalation severity = %v", m.events.events[2].Sev)
	}
}

func TestRecordEventsSuppressesFlapping(t *testing.T) {
	m := eventModel()
	now := t0
	m.recordEvents("rig", cpuInfo(10), nil, now)
	for i := 1; i <= 6; i++ { // oscillate across the threshold every 5 s
		pct := 90.0
		if i%2 == 0 {
			pct = 50
		}
		m.recordEvents("rig", cpuInfo(pct), nil, now.Add(time.Duration(i)*5*time.Second))
	}
	if n := len(m.events.events); n > 3 {
		t.Fatalf("flapping produced %d events, want the cooldown to hold it to a few: %v", n, texts(m))
	}
}

func TestRecordEventsInsightsAppearAndResolve(t *testing.T) {
	m := eventModel()
	in := Insight{Key: "throttle:gpu0", Label: "GPU 0 throttling", Kind: eventThrottle, Sev: SevWarn, Text: "GPU 0 throttled: thermal slowdown"}
	m.recordEvents("rig", cpuInfo(1), nil, t0)
	m.recordEvents("rig", cpuInfo(1), []Insight{in}, t0.Add(10*time.Second))
	m.recordEvents("rig", cpuInfo(1), []Insight{in}, t0.Add(20*time.Second)) // unchanged
	m.recordEvents("rig", cpuInfo(1), nil, t0.Add(3*time.Minute))
	got := strings.Join(texts(m), "|")
	want := "info:monitoring started|throttle:GPU 0 throttled: thermal slowdown|clear:GPU 0 throttling resolved"
	if got != want {
		t.Fatalf("events =\n %s\nwant\n %s", got, want)
	}
}

func TestRecordFailureDeclaresOutageAfterTwoMissesAndRecovery(t *testing.T) {
	m := eventModel()
	m.recordEvents("rig", cpuInfo(1), nil, t0)
	m.recordFailure("rig", nil, t0.Add(10*time.Second))
	if len(m.events.events) != 1 {
		t.Fatalf("one miss logged an event: %v", texts(m))
	}
	m.recordFailure("rig", nil, t0.Add(15*time.Second))
	m.recordFailure("rig", nil, t0.Add(20*time.Second)) // still down: no duplicate
	m.recordEvents("rig", cpuInfo(1), nil, t0.Add(95*time.Second))
	got := texts(m)
	if len(got) != 3 || got[1] != "offline:connection lost" || got[2] != "online:reachable again after 1m 20s" {
		t.Fatalf("events = %v", got)
	}
	if m.events.events[1].Sev != SevCrit {
		t.Errorf("outage severity = %v, want crit", m.events.events[1].Sev)
	}
}

func TestEventLogPersistsAndTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "events.jsonl")
	l := openEventLog(path)
	l.add(Event{Time: t0, Host: "a", Kind: eventAlert, Sev: SevWarn, Text: "CPU at 90% — warning"})
	l.add(Event{Time: t0.Add(time.Minute), Host: "b", Kind: eventClear, Text: "CPU back to normal"})

	again := openEventLog(path)
	if len(again.events) != 2 || again.events[0].Host != "a" || again.events[0].Sev != SevWarn || !again.events[1].Time.Equal(t0.Add(time.Minute)) {
		t.Fatalf("reloaded = %+v", again.events)
	}

	// Past the file cap, opening rewrites it down to the load limit.
	for i := 0; i < eventsFileMaxLines+10; i++ {
		l.add(Event{Time: t0, Host: "h", Kind: eventInfo, Text: "x"})
	}
	trimmed := openEventLog(path)
	if len(trimmed.events) != eventsLoadLimit {
		t.Fatalf("loaded %d events, want %d", len(trimmed.events), eventsLoadLimit)
	}
	if n := len(openEventLog(path).events); n != eventsLoadLimit {
		t.Fatalf("after rewrite file holds %d events, want %d", n, eventsLoadLimit)
	}
}

func TestEventLogIgnoresCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	l := openEventLog(path)
	l.add(Event{Time: t0, Host: "a", Kind: eventInfo, Text: "ok"})
	if err := appendRaw(path, "{not json\n\n"); err != nil {
		t.Fatal(err)
	}
	if n := len(openEventLog(path).events); n != 1 {
		t.Fatalf("got %d events, want the corrupt line skipped", n)
	}
}

func TestTimelineScreenOpensScrollsFiltersAndCloses(t *testing.T) {
	m := eventModel()
	m.screen = ScreenDashboard
	m.width, m.height = 120, 24
	for i := 0; i < 30; i++ {
		host := "alpha"
		if i%2 == 1 {
			host = "beta"
		}
		m.events.add(Event{Time: t0.Add(time.Duration(i) * time.Minute), Host: host, Kind: eventAlert, Sev: SevWarn, Text: "event " + string(rune('A'+i%26))})
	}

	press := func(m Model, key string) Model {
		var msg tea.KeyMsg
		switch key {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		nm, _ := m.Update(msg)
		return nm.(Model)
	}

	m = press(m, "e")
	if m.screen != ScreenTimeline {
		t.Fatalf("screen = %v after e, want timeline", m.screen)
	}
	view := m.View()
	if !strings.Contains(view, "30 events") || !strings.Contains(view, "alpha") {
		t.Fatalf("timeline view missing summary or hosts:\n%s", view)
	}
	if !strings.Contains(view, "older") {
		t.Fatalf("expected an 'older' scroll hint with 30 events in a short terminal:\n%s", view)
	}

	m = press(m, "down")
	if m.timelineScroll != 1 {
		t.Fatalf("scroll = %d, want 1", m.timelineScroll)
	}
	m = press(m, "f")
	if m.timelineHost != "alpha" || len(m.timelineEvents()) != 15 || m.timelineScroll != 0 {
		t.Fatalf("filter = %q with %d events (scroll %d), want alpha/15/0", m.timelineHost, len(m.timelineEvents()), m.timelineScroll)
	}
	m = press(m, "f")
	m = press(m, "f")
	if m.timelineHost != "" {
		t.Fatalf("filter = %q after cycling past the last host, want all", m.timelineHost)
	}
	m = press(m, "esc")
	if m.screen != ScreenDashboard {
		t.Fatalf("screen = %v after esc, want the dashboard we came from", m.screen)
	}
}

func TestTimelineEmptyState(t *testing.T) {
	m := eventModel()
	m.screen = ScreenTimeline
	m.width, m.height = 100, 20
	if !strings.Contains(m.View(), "nothing yet") {
		t.Fatalf("empty timeline should explain itself:\n%s", m.View())
	}
}

func appendRaw(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(s)
	return err
}
