package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
)

// Event kinds.
const (
	eventInfo     = "info"
	eventAlert    = "alert"
	eventClear    = "clear"
	eventOffline  = "offline"
	eventOnline   = "online"
	eventThrottle = "throttle"
	eventForecast = "forecast"
)

// Event is one entry in the timeline: something worth reading about when you
// come back to the screen after hours away.
type Event struct {
	Time time.Time `json:"t"`
	Host string    `json:"host"`
	Kind string    `json:"kind"`
	Sev  Severity  `json:"sev"`
	Text string    `json:"text"`
}

const (
	// eventsLoadLimit is how many past events a new session shows.
	eventsLoadLimit = 1000
	// eventsFileMaxLines bounds the log file; past it, the file is rewritten to
	// the most recent eventsLoadLimit lines on the next start.
	eventsFileMaxLines = 4000
	// eventCooldown suppresses repeats of the same event, so a reading hovering
	// on a threshold doesn't fill the log with alternating alert/recover lines.
	eventCooldown = 60 * time.Second
)

// eventLog is an append-only JSONL log with an in-memory copy. An empty path
// keeps it memory-only, which is what tests and an unwritable config directory
// get.
type eventLog struct {
	path   string
	events []Event // oldest first
	last   map[string]time.Time
}

func eventLogPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "rigwatch", "events.jsonl")
}

// openEventLog loads the tail of the log at path (if any).
func openEventLog(path string) *eventLog {
	l := &eventLog{path: path, last: map[string]time.Time{}}
	if path == "" {
		return l
	}
	f, err := os.Open(path)
	if err != nil {
		return l
	}
	defer func() { _ = f.Close() }()

	var all []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var ev Event
		if json.Unmarshal(scanner.Bytes(), &ev) == nil && ev.Text != "" {
			all = append(all, ev)
		}
	}
	trim := len(all) > eventsFileMaxLines
	if len(all) > eventsLoadLimit {
		all = all[len(all)-eventsLoadLimit:]
	}
	l.events = all
	if trim {
		l.rewrite()
	}
	return l
}

func (l *eventLog) rewrite() {
	if l.path == "" {
		return
	}
	var b strings.Builder
	for _, ev := range l.events {
		if line, err := json.Marshal(ev); err == nil {
			b.Write(line)
			b.WriteByte('\n')
		}
	}
	_ = os.WriteFile(l.path, []byte(b.String()), 0600)
}

// add records ev. Persistence is best-effort: a monitor should never fail
// because its history could not be written.
func (l *eventLog) add(ev Event) {
	l.events = append(l.events, ev)
	if len(l.events) > eventsLoadLimit*2 {
		l.events = l.events[len(l.events)-eventsLoadLimit:]
	}
	if l.path == "" {
		return
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

// addThrottled records ev unless an event with the same key was logged within
// the cooldown. It reports whether the event was written.
func (l *eventLog) addThrottled(key string, ev Event) bool {
	if prev, ok := l.last[key]; ok && ev.Time.Sub(prev) < eventCooldown {
		return false
	}
	l.last[key] = ev.Time
	l.add(ev)
	return true
}

// eventState is the per-host memory recordEvents compares each poll against, to
// turn "what is true now" into "what changed".
type eventState struct {
	started  bool
	alerts   map[string]Severity
	insights map[string]Insight
	failures int
	offline  time.Time // when the host was declared unreachable; zero if reachable
}

func (m *Model) hostEventState(host string) *eventState {
	if m.eventStates == nil {
		m.eventStates = map[string]*eventState{}
	}
	st := m.eventStates[host]
	if st == nil {
		st = &eventState{alerts: map[string]Severity{}, insights: map[string]Insight{}}
		m.eventStates[host] = st
	}
	return st
}

// recordEvents logs what changed on host since the previous poll: alerts that
// began, worsened or cleared, and insights that appeared or went away.
func (m *Model) recordEvents(host string, info *internal.SystemInfo, insights []Insight, now time.Time) {
	if m.events == nil {
		m.events = openEventLog("")
	}
	st := m.hostEventState(host)

	if !st.offline.IsZero() {
		m.events.add(Event{Time: now, Host: host, Kind: eventOnline, Text: "reachable again after " + formatDowntime(now.Sub(st.offline))})
		st.offline = time.Time{}
	}
	st.failures = 0

	if !st.started {
		st.started = true
		m.events.add(Event{Time: now, Host: host, Kind: eventInfo, Text: "monitoring started"})
	}

	active := map[string]Alert{}
	for _, a := range hostAlerts(info, m.settings.Thresholds) {
		active[a.Metric] = a
		prev, was := st.alerts[a.Metric]
		if was && a.Sev <= prev {
			st.alerts[a.Metric] = a.Sev // a downgrade re-arms the escalation
			continue
		}
		st.alerts[a.Metric] = a.Sev
		level := "warning"
		if a.Sev == SevCrit {
			level = "critical"
		}
		m.events.addThrottled(host+"|alert|"+a.Metric+"|"+level, Event{
			Time: now, Host: host, Kind: eventAlert, Sev: a.Sev,
			Text: fmt.Sprintf("%s at %.0f%s — %s", a.Metric, a.Value, a.Unit, level),
		})
	}
	for metric := range st.alerts {
		if _, still := active[metric]; !still {
			delete(st.alerts, metric)
			m.events.addThrottled(host+"|clear|"+metric, Event{
				Time: now, Host: host, Kind: eventClear, Text: metric + " back to normal",
			})
		}
	}

	current := map[string]Insight{}
	for _, in := range insights {
		current[in.Key] = in
		prev, was := st.insights[in.Key]
		if was && in.Sev <= prev.Sev {
			st.insights[in.Key] = in // keep the latest wording without re-logging
			continue
		}
		st.insights[in.Key] = in
		m.events.addThrottled(host+"|insight|"+in.Key, Event{Time: now, Host: host, Kind: in.Kind, Sev: in.Sev, Text: in.Text})
	}
	for key, prev := range st.insights {
		if _, still := current[key]; !still {
			delete(st.insights, key)
			m.events.addThrottled(host+"|resolved|"+key, Event{Time: now, Host: host, Kind: eventClear, Text: prev.Label + " resolved"})
		}
	}
}

// recordFailure notes a failed poll. One miss is routine on a flaky link; two
// in a row is declared an outage.
func (m *Model) recordFailure(host string, err error, now time.Time) {
	if m.events == nil {
		m.events = openEventLog("")
	}
	st := m.hostEventState(host)
	st.failures++
	if st.failures == 2 && st.offline.IsZero() {
		st.offline = now
		text := "connection lost"
		if err != nil {
			text += ": " + truncateVisible(err.Error(), 80)
		}
		m.events.add(Event{Time: now, Host: host, Kind: eventOffline, Sev: SevCrit, Text: text})
	}
}

func formatDowntime(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// -- timeline screen --

func (m *Model) openTimeline() {
	m.timelinePrev = m.screen
	m.timelineScroll = 0
	m.timelineHost = ""
	m.screen = ScreenTimeline
}

// timelineEvents returns the events to show, newest first, honoring the host
// filter.
func (m Model) timelineEvents() []Event {
	if m.events == nil {
		return nil
	}
	out := make([]Event, 0, len(m.events.events))
	for i := len(m.events.events) - 1; i >= 0; i-- {
		ev := m.events.events[i]
		if m.timelineHost == "" || ev.Host == m.timelineHost {
			out = append(out, ev)
		}
	}
	return out
}

func (m Model) timelineVisibleRows() int {
	return max(3, m.height-9)
}

func (m Model) updateTimeline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	events := m.timelineEvents()
	maxScroll := max(0, len(events)-m.timelineVisibleRows())
	page := max(1, m.timelineVisibleRows()-1)
	switch msg.String() {
	case "esc", "e", "q":
		m.screen = m.timelinePrev
		if m.screen == ScreenTimeline {
			m.screen = ScreenDashboard
		}
	case "up", "k":
		m.timelineScroll = max(0, m.timelineScroll-1)
	case "down", "j":
		m.timelineScroll = min(maxScroll, m.timelineScroll+1)
	case "pgup", "b":
		m.timelineScroll = max(0, m.timelineScroll-page)
	case "pgdown", "space", " ":
		m.timelineScroll = min(maxScroll, m.timelineScroll+page)
	case "home":
		m.timelineScroll = 0
	case "end":
		m.timelineScroll = maxScroll
	case "f":
		m.cycleTimelineHost()
		m.timelineScroll = 0
	}
	return m, nil
}

// cycleTimelineHost steps the filter through all hosts and then back to "all".
func (m *Model) cycleTimelineHost() {
	var names []string
	seen := map[string]bool{}
	if m.events != nil {
		for _, ev := range m.events.events {
			if !seen[ev.Host] {
				seen[ev.Host] = true
				names = append(names, ev.Host)
			}
		}
	}
	if m.timelineHost == "" {
		if len(names) > 0 {
			m.timelineHost = names[0]
		}
		return
	}
	for i, n := range names {
		if n == m.timelineHost {
			if i+1 < len(names) {
				m.timelineHost = names[i+1]
			} else {
				m.timelineHost = ""
			}
			return
		}
	}
	m.timelineHost = ""
}

func eventGlyph(ev Event) string {
	switch ev.Kind {
	case eventOffline:
		return dangerStyle.Render("✕")
	case eventOnline:
		return successStyle.Render("✓")
	case eventClear:
		return successStyle.Render("●")
	case eventInfo:
		return mutedStyle.Render("·")
	case eventThrottle, eventForecast:
		if ev.Sev == SevOK {
			return accentStyle.Render("◇")
		}
	}
	return severityGlyph(ev.Sev)
}

func formatEventTime(t, now time.Time) string {
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 02 15:04")
}

func (m Model) renderTimeline() string {
	var b strings.Builder
	filter := "all hosts"
	if m.timelineHost != "" {
		filter = m.timelineHost
	}
	subtitle := fmt.Sprintf("showing %s  •  ↑/↓ scroll  •  pgup/pgdn page  •  f filter host  •  e or esc back  •  q back", filter)
	b.WriteString(renderHeroHeader("RIGWATCH // TIMELINE", subtitle, m.width, m.animationFrame))
	b.WriteString("\n\n")

	events := m.timelineEvents()
	width := clampInt(m.width-2, 50, 160)

	if len(events) == 0 {
		b.WriteString(renderPanel("EVENTS", mutedStyle.Render("nothing yet — alerts, throttling, forecasts and outages will be logged here"), width))
		return clampToHeight(b.String(), m.height)
	}

	var crit, warn int
	for _, ev := range events {
		switch ev.Sev {
		case SevCrit:
			crit++
		case SevWarn:
			warn++
		}
	}
	summary := fmt.Sprintf("%d events", len(events))
	if crit > 0 {
		summary += "  " + dangerStyle.Render(fmt.Sprintf("■ %d critical", crit))
	}
	if warn > 0 {
		summary += "  " + warningStyle.Render(fmt.Sprintf("▲ %d warning", warn))
	}

	rows := m.timelineVisibleRows()
	start := min(m.timelineScroll, max(0, len(events)-rows))
	end := min(len(events), start+rows)

	now := time.Now()
	hostWidth := 0
	for _, ev := range events[start:end] {
		hostWidth = max(hostWidth, len(ev.Host))
	}
	hostWidth = min(hostWidth, 16)
	textWidth := max(width-hostWidth-22, 16)

	lines := make([]string, 0, rows)
	for _, ev := range events[start:end] {
		style := severityStyle(ev.Sev, panelTextStyle)
		if ev.Kind == eventClear || ev.Kind == eventInfo {
			style = mutedStyle
		}
		lines = append(lines, fmt.Sprintf("%s  %s  %s %s",
			mutedStyle.Render(fmt.Sprintf("%-12s", formatEventTime(ev.Time, now))),
			accentStyle.Render(fmt.Sprintf("%-*s", hostWidth, truncateVisible(ev.Host, hostWidth))),
			eventGlyph(ev),
			style.Render(truncateVisible(ev.Text, textWidth))))
	}
	more := ""
	if end < len(events) {
		more = mutedStyle.Render(fmt.Sprintf("  ↓ %d older", len(events)-end))
	}
	b.WriteString(summary + more + "\n")
	b.WriteString(renderPanel("EVENTS", strings.Join(lines, "\n"), width))
	return clampToHeight(b.String(), m.height)
}
