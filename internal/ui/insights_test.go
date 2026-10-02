package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// feed records n samples 30 s apart, value(i) each, and returns the time of the last.
func feed(ts *trendStore, key string, n int, value func(i int) float64) time.Time {
	var at time.Time
	for i := 0; i < n; i++ {
		at = t0.Add(time.Duration(i) * forecastSampleEvery)
		ts.add(key, at, value(i))
	}
	return at
}

func TestLinearFitFindsSlopeAndRejectsNoise(t *testing.T) {
	ts := newTrendStore()
	feed(ts, "a", 40, func(i int) float64 { return 100 + 2*float64(i) }) // +2 per 30 s
	slope, r2 := linearFit(ts.series["a"])
	if slope < 0.0666 || slope > 0.0667 || r2 < 0.999 {
		t.Fatalf("slope=%v r2=%v, want ~0.0667/s and r2≈1", slope, r2)
	}
	flat := newTrendStore()
	feed(flat, "a", 40, func(i int) float64 { return 5 })
	if _, r2 := linearFit(flat.series["a"]); r2 != 0 {
		t.Fatalf("flat series r2=%v, want 0", r2)
	}
	noisy := newTrendStore()
	feed(noisy, "a", 40, func(i int) float64 { return float64((i * 7919) % 13) })
	if _, r2 := linearFit(noisy.series["a"]); r2 >= forecastMinR2 {
		t.Fatalf("noise passed as a trend, r2=%v", r2)
	}
}

func TestTrendStoreSamplesSparselyAndTrims(t *testing.T) {
	ts := newTrendStore()
	ts.add("a", t0, 1)
	ts.add("a", t0.Add(5*time.Second), 2) // too soon
	if len(ts.series["a"]) != 1 {
		t.Fatalf("got %d samples, want 1 (second was inside the sample interval)", len(ts.series["a"]))
	}
	ts.add("a", t0.Add(forecastRetention+time.Minute), 3)
	if len(ts.series["a"]) != 1 || ts.series["a"][0].v != 3 {
		t.Fatalf("old sample not trimmed: %+v", ts.series["a"])
	}
}

func TestForecastNeedsHistoryBeforeSpeaking(t *testing.T) {
	ts := newTrendStore()
	now := feed(ts, "ram", 6, func(i int) float64 { return 1000 + 500*float64(i) }) // only 2.5 min
	info := &internal.SystemInfo{RAM: internal.RAMInfo{Total: 64000, Used: 3500}}
	if got := forecastInsights(ts, info, now); len(got) != 0 {
		t.Fatalf("forecast from 2.5 minutes of data: %+v", got)
	}
}

func TestForecastFlagsRAMGrowth(t *testing.T) {
	ts := newTrendStore()
	// +100 MB per 30 s = 200 MB/min, over 20 minutes, with 8 GB left.
	now := feed(ts, "ram", 41, func(i int) float64 { return 50000 + 100*float64(i) })
	used := 50000 + 100*40
	info := &internal.SystemInfo{RAM: internal.RAMInfo{Total: used + 8000, Used: used}}
	got := forecastInsights(ts, info, now)
	if len(got) != 1 || got[0].Key != "forecast:ram" {
		t.Fatalf("got %+v, want one RAM forecast", got)
	}
	if !strings.Contains(got[0].Text, "+200 MB/min") || !strings.Contains(got[0].Text, "~40m") {
		t.Fatalf("text = %q, want 200 MB/min and ~40m", got[0].Text)
	}
	if got[0].Sev != SevWarn { // under 2 h but not under 20 min
		t.Fatalf("sev = %v, want warn", got[0].Sev)
	}
}

func TestForecastIgnoresSteadyRAMAndTinyDrift(t *testing.T) {
	ts := newTrendStore()
	now := feed(ts, "ram", 41, func(i int) float64 { return 20000 + float64(i)*0.1 }) // 0.2 MB/min
	info := &internal.SystemInfo{RAM: internal.RAMInfo{Total: 64000, Used: 20004}}
	if got := forecastInsights(ts, info, now); len(got) != 0 {
		t.Fatalf("flagged drift: %+v", got)
	}
}

func TestForecastFlagsFillingDisk(t *testing.T) {
	ts := newTrendStore()
	const gb = float64(1 << 30)
	// +1 GB per 30 s → 120 GB/h, 100 GB free at the end.
	now := feed(ts, "disk:/", 41, func(i int) float64 { return 400*gb + float64(i)*gb })
	used := uint64(440 * gb)
	info := &internal.SystemInfo{Disk: []internal.DiskInfo{{MountPoint: "/", TotalBytes: used + uint64(100*gb), UsedBytes: used}}}
	got := forecastInsights(ts, info, now)
	if len(got) != 1 || !strings.Contains(got[0].Text, "disk /") || !strings.Contains(got[0].Text, "full in ~50m") {
		t.Fatalf("got %+v, want a disk forecast of ~50m", got)
	}
	if got[0].Sev != SevCrit { // under 12 h
		t.Fatalf("sev = %v, want crit", got[0].Sev)
	}
}

func TestThrottleInsightsGPU(t *testing.T) {
	info := &internal.SystemInfo{GPUs: []internal.GPUInfo{
		{Index: "0", ClockMHz: 1400, MaxClockMHz: 2000, Throttle: []string{"thermal slowdown"}},
		{Index: "1", ClockMHz: 1990, MaxClockMHz: 2000, Throttle: []string{"power cap"}}, // normal at full load
		{Index: "2", ClockMHz: 1200, MaxClockMHz: 2000, Throttle: []string{"power cap"}},
		{Index: "3", ClockMHz: 900, MaxClockMHz: 2000, Throttle: []string{"hw thermal slowdown"}},
	}}
	got := throttleInsights(info)
	if len(got) != 3 {
		t.Fatalf("got %d insights, want 3 (GPU 1 at full clocks is not news): %+v", len(got), got)
	}
	byKey := map[string]Insight{}
	for _, in := range got {
		byKey[in.Key] = in
	}
	if byKey["throttle:gpu0"].Sev != SevWarn || !strings.Contains(byKey["throttle:gpu0"].Text, "1400/2000 MHz (70%)") {
		t.Errorf("gpu0 = %+v", byKey["throttle:gpu0"])
	}
	if byKey["throttle:gpu2"].Sev != SevOK || !strings.Contains(byKey["throttle:gpu2"].Text, "power-limited") {
		t.Errorf("gpu2 = %+v, want informational power-limited", byKey["throttle:gpu2"])
	}
	if byKey["throttle:gpu3"].Sev != SevCrit {
		t.Errorf("gpu3 = %+v, want critical", byKey["throttle:gpu3"])
	}
}

func TestThrottleInsightsCPURequiresLoadAndHeat(t *testing.T) {
	hot := &internal.SystemInfo{
		CPU:   internal.CPUInfo{UsagePercent: 90, FreqMHz: 2800, MaxFreqMHz: 5200},
		Temps: []internal.TemperatureInfo{{Name: "CPU", Celsius: 94}},
	}
	got := throttleInsights(hot)
	if len(got) != 1 || got[0].Sev != SevWarn || !strings.Contains(got[0].Text, "thermal throttling") {
		t.Fatalf("hot loaded CPU: %+v", got)
	}
	idle := &internal.SystemInfo{CPU: internal.CPUInfo{UsagePercent: 3, FreqMHz: 800, MaxFreqMHz: 5200}}
	if got := throttleInsights(idle); len(got) != 0 {
		t.Fatalf("idle CPU at low clocks flagged: %+v", got)
	}
	cool := &internal.SystemInfo{
		CPU:   internal.CPUInfo{UsagePercent: 70, FreqMHz: 3000, MaxFreqMHz: 5200},
		Temps: []internal.TemperatureInfo{{Name: "CPU", Celsius: 60}},
	}
	if got := throttleInsights(cool); len(got) != 0 {
		t.Fatalf("cool CPU at 58%% clocks flagged: %+v", got)
	}
}

func TestFormatETA(t *testing.T) {
	cases := map[time.Duration]string{30 * time.Second: "<1m", 42 * time.Minute: "42m", 3*time.Hour + 10*time.Minute: "3h 10m", 20 * time.Hour: "20h", 72 * time.Hour: "3d"}
	for d, want := range cases {
		if got := formatETA(d); got != want {
			t.Errorf("formatETA(%v) = %s, want %s", d, got, want)
		}
	}
}

func TestRateGPUProcessesDerivesUtilFromBusyTime(t *testing.T) {
	prev := []internal.GPUProcessInfo{{PID: 1, GPU: "c4", EngineNs: 1_000_000_000, UtilPct: -1}}
	cur := []internal.GPUProcessInfo{
		{PID: 2, GPU: "c4", VRAMMB: 900, EngineNs: 5, UtilPct: -1},                             // new: unknown
		{PID: 1, GPU: "c4", VRAMMB: 100, EngineNs: 1_000_000_000 + 2_500_000_000, UtilPct: -1}, // 2.5 s busy in 5 s
		{PID: 3, GPU: "0", VRAMMB: 50, UtilPct: 80},                                            // driver-reported
	}
	got := rateGPUProcesses(prev, cur, 5)
	if got[0].PID != 3 || got[1].PID != 1 || got[2].PID != 2 {
		t.Fatalf("order = %d,%d,%d, want busiest first (3,1,2)", got[0].PID, got[1].PID, got[2].PID)
	}
	if got[1].UtilPct != 50 {
		t.Errorf("pid 1 util = %v, want 50", got[1].UtilPct)
	}
	if got[2].UtilPct != -1 {
		t.Errorf("new process util = %v, want unknown (-1)", got[2].UtilPct)
	}
}

// End to end through the model: a RAM series that climbs for twenty minutes
// ends up as an insight on the host's history and as an event in the log.
func TestRecordInsightsFeedsHistoryAndEventLog(t *testing.T) {
	m := InitialModel(nil, 5*time.Second)
	m.settings = DefaultSettings()
	m.events = openEventLog("")

	var info *internal.SystemInfo
	for i := 0; i <= 40; i++ {
		used := 50000 + 100*i
		info = &internal.SystemInfo{RAM: internal.RAMInfo{Total: 58100 + 100*i, Used: used, UsagePercent: float64(used) / float64(58100+100*i) * 100}}
		m.recordInsights("rig", info, t0.Add(time.Duration(i)*forecastSampleEvery))
	}
	ins := m.metricHistories["rig"].Insights
	if len(ins) != 1 || ins[0].Key != "forecast:ram" {
		t.Fatalf("history insights = %+v, want the RAM forecast", ins)
	}
	var logged bool
	for _, ev := range m.events.events {
		if ev.Kind == eventForecast && strings.Contains(ev.Text, "RAM +200 MB/min") {
			logged = true
		}
	}
	if !logged {
		t.Fatalf("forecast not in event log: %v", texts(m))
	}
}

func TestInsightsPanelRenders(t *testing.T) {
	empty := renderInsightsSection(nil, 60)
	if !strings.Contains(empty, "INSIGHTS") || !strings.Contains(empty, "no throttling") {
		t.Fatalf("empty panel:\n%s", empty)
	}
	full := renderInsightsSection([]Insight{{Text: "GPU 0 throttled: thermal slowdown", Sev: SevWarn}, {Text: "RAM +90 MB/min, full in ~5h 00m", Sev: SevOK}}, 70)
	if !strings.Contains(full, "thermal slowdown") || !strings.Contains(full, "full in ~5h") {
		t.Fatalf("panel missing insights:\n%s", full)
	}
}

func TestGPUProcessPanelAndClockLine(t *testing.T) {
	panel := renderGPUProcessSection([]internal.GPUProcessInfo{
		{PID: 4242, Name: "python3", VRAMMB: 8123, UtilPct: 87, GPU: "0"},
		{PID: 7, Name: "Xorg", VRAMMB: 0, UtilPct: -1, GPU: "1"},
	}, 70)
	for _, want := range []string{"GPU PROCESSES", "python3", "7.9 GB", "87%", "Xorg", "GPU"} {
		if !strings.Contains(panel, want) {
			t.Errorf("panel missing %q:\n%s", want, panel)
		}
	}
	if !strings.Contains(renderGPUProcessSection(nil, 60), "no GPU clients") {
		t.Error("empty state missing")
	}
	line := renderGPUClockLine([]internal.GPUInfo{
		{ClockMHz: 1900, MaxClockMHz: 2000},
		{ClockMHz: 1000, MaxClockMHz: 2000, Throttle: []string{"thermal slowdown"}},
	}, 20)
	if !strings.Contains(line, "1000/2000 MHz") || !strings.Contains(line, "thermal slowdown") {
		t.Errorf("clock line should report the slowest GPU with its reason: %q", line)
	}
	if renderGPUClockLine([]internal.GPUInfo{{}}, 20) != "" {
		t.Error("no clock data should render nothing")
	}
}
