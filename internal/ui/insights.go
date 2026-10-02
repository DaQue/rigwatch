package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

// Insight is a finding derived from several readings or from their history,
// rather than a single threshold crossing: a throttled clock, or a trend that
// will run a resource out. Severity SevOK means informational.
type Insight struct {
	Key   string // stable identity, so appearing and clearing can be logged
	Label string // short noun phrase for the "resolved" log line
	Text  string
	Sev   Severity
	Kind  string // eventThrottle or eventForecast
}

const (
	// forecastSampleEvery is the spacing of trend samples. The dashboard polls
	// every few seconds, but disks and leaks move over minutes, and a sparse
	// series keeps six hours of history small and the fit insensitive to jitter.
	forecastSampleEvery = 30 * time.Second
	forecastRetention   = 6 * time.Hour
	// forecastWindow is how far back a fit looks: long enough to see a steady
	// trend, short enough to notice when it changes.
	forecastWindow = 30 * time.Minute
	// forecastMinSpan is the least history a forecast will speak from. A line
	// fitted to two minutes of data predicts anything.
	forecastMinSpan    = 10 * time.Minute
	forecastMinSamples = 8
	// forecastMinR2 is how straight the trend must be. Real leaks and filling
	// disks are close to linear; a noisy series that happens to slope is not worth
	// an alert.
	forecastMinR2 = 0.8
)

type trendSample struct {
	at time.Time
	v  float64
}

// trendStore keeps long-horizon, down-sampled series per host for forecasting.
// It lives only for the session; nothing about it is persisted.
type trendStore struct {
	series map[string][]trendSample
}

func newTrendStore() *trendStore { return &trendStore{series: map[string][]trendSample{}} }

// add records v for key unless the previous sample is too recent, and drops
// anything beyond the retention horizon.
func (t *trendStore) add(key string, at time.Time, v float64) {
	s := t.series[key]
	if n := len(s); n > 0 && at.Sub(s[n-1].at) < forecastSampleEvery {
		return
	}
	s = append(s, trendSample{at, v})
	cut := 0
	for cut < len(s) && at.Sub(s[cut].at) > forecastRetention {
		cut++
	}
	t.series[key] = s[cut:]
}

// recent returns the samples inside the forecast window ending at now.
func (t *trendStore) recent(key string, now time.Time) []trendSample {
	s := t.series[key]
	i := 0
	for i < len(s) && now.Sub(s[i].at) > forecastWindow {
		i++
	}
	return s[i:]
}

// linearFit returns the least-squares slope in units per second and the R² of
// the fit. A flat series reports r2 = 0, so it never passes as a trend.
func linearFit(samples []trendSample) (slope, r2 float64) {
	n := float64(len(samples))
	if n < 2 {
		return 0, 0
	}
	t0 := samples[0].at
	var sx, sy, sxx, sxy, syy float64
	for _, p := range samples {
		x := p.at.Sub(t0).Seconds()
		sx += x
		sy += p.v
		sxx += x * x
		sxy += x * p.v
		syy += p.v * p.v
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0, 0
	}
	slope = (n*sxy - sx*sy) / den
	varY := n*syy - sy*sy
	if varY <= 0 {
		return slope, 0
	}
	cov := n*sxy - sx*sy
	return slope, (cov * cov) / (den * varY)
}

// trendOf fits the recent window for key, returning ok only when there is
// enough history and the series is straight enough to extrapolate.
func (t *trendStore) trendOf(key string, now time.Time) (slope float64, ok bool) {
	s := t.recent(key, now)
	if len(s) < forecastMinSamples || s[len(s)-1].at.Sub(s[0].at) < forecastMinSpan {
		return 0, false
	}
	slope, r2 := linearFit(s)
	return slope, r2 >= forecastMinR2
}

// record feeds one poll into the trend store.
func (t *trendStore) record(info *internal.SystemInfo, at time.Time) {
	if info.RAM.Total > 0 {
		t.add("ram", at, float64(info.RAM.Used))
	}
	for _, d := range info.Disk {
		if d.TotalBytes > 0 {
			t.add("disk:"+d.MountPoint, at, float64(d.UsedBytes))
		}
	}
}

// formatETA renders a duration as a short "in 3h 10m" style span.
func formatETA(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		if h >= 10 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh %02dm", h, m)
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// forecastInsights projects RAM and each disk forward and reports the ones on
// course to fill within a horizon that still matters.
func forecastInsights(t *trendStore, info *internal.SystemInfo, now time.Time) []Insight {
	var out []Insight

	if info.RAM.Total > 0 {
		if slope, ok := t.trendOf("ram", now); ok && slope > 0 {
			perMin := slope * 60
			span := forecastWindow.Seconds()
			// Ignore creep: a rate that adds under 256 MB (or 1.5% of RAM) over
			// the window is allocator noise, not a leak.
			if slope*span >= math.Max(256, 0.015*float64(info.RAM.Total)) {
				left := float64(info.RAM.Total - info.RAM.Used)
				eta := time.Duration(left / slope * float64(time.Second))
				if eta > 0 && eta <= 24*time.Hour {
					sev := SevOK
					if eta < 2*time.Hour {
						sev = SevWarn
					}
					if eta < 20*time.Minute {
						sev = SevCrit
					}
					out = append(out, Insight{
						Key: "forecast:ram", Label: "RAM growth", Kind: eventForecast, Sev: sev,
						Text: fmt.Sprintf("RAM +%.0f MB/min, full in ~%s", perMin, formatETA(eta)),
					})
				}
			}
		}
	}

	for _, d := range info.Disk {
		if d.TotalBytes == 0 {
			continue
		}
		slope, ok := t.trendOf("disk:"+d.MountPoint, now) // bytes per second
		if !ok || slope <= 0 {
			continue
		}
		// Under 50 MB over the window is log churn; not worth a forecast.
		if slope*forecastWindow.Seconds() < 50*1024*1024 {
			continue
		}
		left := float64(d.TotalBytes - d.UsedBytes)
		eta := time.Duration(left / slope * float64(time.Second))
		if eta <= 0 || eta > 30*24*time.Hour {
			continue
		}
		sev := SevOK
		if eta < 3*24*time.Hour {
			sev = SevWarn
		}
		if eta < 12*time.Hour {
			sev = SevCrit
		}
		out = append(out, Insight{
			Key: "forecast:disk:" + d.MountPoint, Label: "disk " + d.MountPoint + " growth", Kind: eventForecast, Sev: sev,
			Text: fmt.Sprintf("disk %s +%s/h, full in ~%s", d.MountPoint, humanBytes(slope*3600), formatETA(eta)),
		})
	}
	return out
}

// humanBytes formats a byte count in binary units.
func humanBytes(b float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for b >= 1024 && i < len(units)-1 {
		b /= 1024
		i++
	}
	if b >= 100 || i == 0 {
		return fmt.Sprintf("%.0f %s", b, units[i])
	}
	return fmt.Sprintf("%.1f %s", b, units[i])
}

// throttleInsights reports GPUs and the CPU running well below their top clocks
// for a reason worth knowing about.
func throttleInsights(info *internal.SystemInfo) []Insight {
	var out []Insight

	for _, g := range info.GPUs {
		if len(g.Throttle) == 0 {
			continue
		}
		pct := 100
		if g.MaxClockMHz > 0 {
			pct = g.ClockMHz * 100 / g.MaxClockMHz
		}
		onlyPower := len(g.Throttle) == 1 && g.Throttle[0] == "power cap"
		// Sitting at the power cap is how a busy GPU normally behaves. Mention it
		// only when it is actually costing clocks.
		if onlyPower && (g.MaxClockMHz == 0 || pct > 92) {
			continue
		}
		sev := SevWarn
		switch {
		case onlyPower:
			sev = SevOK
		case containsString(g.Throttle, "hw thermal slowdown"), containsString(g.Throttle, "power brake"):
			sev = SevCrit
		}
		text := fmt.Sprintf("GPU %s throttled: %s", g.Index, strings.Join(g.Throttle, ", "))
		if onlyPower {
			text = fmt.Sprintf("GPU %s power-limited", g.Index)
		}
		if g.MaxClockMHz > 0 {
			text += fmt.Sprintf(" · %d/%d MHz (%d%%)", g.ClockMHz, g.MaxClockMHz, pct)
		}
		out = append(out, Insight{Key: "throttle:gpu" + g.Index, Label: "GPU " + g.Index + " throttling", Kind: eventThrottle, Sev: sev, Text: text})
	}

	cpu := info.CPU
	if cpu.MaxFreqMHz > 0 && cpu.FreqMHz > 0 && cpu.UsagePercent >= 60 {
		ratio := float64(cpu.FreqMHz) / float64(cpu.MaxFreqMHz)
		temp := cpuTemp(info)
		switch {
		// Low clocks under load only mean thermal throttling when it is also hot;
		// otherwise a governor or power profile may simply be holding them down.
		case ratio <= 0.65 && temp >= 80:
			out = append(out, Insight{
				Key: "throttle:cpu", Label: "CPU throttling", Kind: eventThrottle, Sev: SevWarn,
				Text: fmt.Sprintf("CPU thermal throttling: %.1f/%.1f GHz at %.0f°C", float64(cpu.FreqMHz)/1000, float64(cpu.MaxFreqMHz)/1000, temp),
			})
		case ratio <= 0.5 && cpu.UsagePercent >= 80:
			out = append(out, Insight{
				Key: "throttle:cpu", Label: "CPU clocks low", Kind: eventThrottle, Sev: SevOK,
				Text: fmt.Sprintf("CPU clocks low under load: %.1f/%.1f GHz (power limit or governor?)", float64(cpu.FreqMHz)/1000, float64(cpu.MaxFreqMHz)/1000),
			})
		}
	}
	return out
}

// cpuTemp is the hottest sensor labelled as the CPU, or 0 when there is none.
func cpuTemp(info *internal.SystemInfo) float64 {
	var hottest float64
	for _, t := range info.Temps {
		if strings.HasPrefix(t.Name, "CPU") && t.Celsius > hottest {
			hottest = t.Celsius
		}
	}
	return hottest
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// computeInsights gathers every derived finding for one poll, worst first.
func computeInsights(t *trendStore, info *internal.SystemInfo, now time.Time) []Insight {
	if info == nil {
		return nil
	}
	insights := append(throttleInsights(info), forecastInsights(t, info, now)...)
	for i := 1; i < len(insights); i++ { // stable insertion sort, worst first
		for j := i; j > 0 && insights[j].Sev > insights[j-1].Sev; j-- {
			insights[j], insights[j-1] = insights[j-1], insights[j]
		}
	}
	return insights
}

// renderInsightsSection renders the INSIGHTS panel for the single-host view.
func renderInsightsSection(insights []Insight, width int) string {
	if len(insights) == 0 {
		return renderPanel("INSIGHTS", successStyle.Render("● no throttling, no trends")+"\n"+mutedStyle.Render("watching clocks, RAM and disk growth"), width)
	}
	var b strings.Builder
	limit := min(len(insights), 5)
	for i := 0; i < limit; i++ {
		in := insights[i]
		if i != 0 {
			b.WriteString("\n")
		}
		glyph := accentStyle.Render("◇")
		if in.Sev != SevOK {
			glyph = severityGlyph(in.Sev)
		}
		b.WriteString(glyph + " " + severityStyle(in.Sev, panelTextStyle).Render(truncateVisible(in.Text, max(width-6, 10))))
	}
	if len(insights) > limit {
		b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("+%d more", len(insights)-limit)))
	}
	return renderPanelSev("INSIGHTS", b.String(), width, insights[0].Sev)
}
