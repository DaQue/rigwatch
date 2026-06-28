package ui

import (
	"strings"
	"testing"

	"github.com/allisonhere/rigwatch/internal"
)

func TestRateDiskIOUsesCounterDelta(t *testing.T) {
	previous := []internal.DiskIOInfo{{Device: "sda", ReadBytes: 1000, WriteBytes: 2000}}
	current := []internal.DiskIOInfo{{Device: "sda", ReadBytes: 5000, WriteBytes: 6000}}

	got := rateDiskIO(previous, current, 2.0) // 2 seconds elapsed
	if len(got) != 1 {
		t.Fatalf("expected 1 device, got %d", len(got))
	}
	if got[0].ReadBps != 2000 || got[0].WriteBps != 2000 {
		t.Fatalf("rates = r%d w%d, want 2000/2000", got[0].ReadBps, got[0].WriteBps)
	}
}

func TestRateDiskIOIgnoresCounterReset(t *testing.T) {
	previous := []internal.DiskIOInfo{{Device: "sda", ReadBytes: 9000, WriteBytes: 9000}}
	current := []internal.DiskIOInfo{{Device: "sda", ReadBytes: 100, WriteBytes: 100}}
	got := rateDiskIO(previous, current, 2.0)
	if got[0].ReadBps != 0 || got[0].WriteBps != 0 {
		t.Fatalf("expected zero rate on counter reset, got r%d w%d", got[0].ReadBps, got[0].WriteBps)
	}
}

func extendedSampleInfo() *internal.SystemInfo {
	info := sampleLargeSystemInfo()
	info.Swap = internal.SwapInfo{Total: 8192, Used: 1024, UsagePercent: 12.5}
	info.Load = internal.LoadInfo{Load1: 1.2, Load5: 0.9, Load15: 0.7, Running: 3, Total: 500}
	info.DiskIO = []internal.DiskIOInfo{{Device: "sda", ReadBps: 1024 * 1024, WriteBps: 512 * 1024}}
	info.Fans = []internal.FanInfo{{Name: "CPU Fan", RPM: 1200}}
	return info
}

func TestExtendedGridIncludesExtraSensors(t *testing.T) {
	got := renderMetricsGrid(extendedSampleInfo(), metricHistory{}, 160, true)
	for _, want := range []string{"SWAP", "LOAD AVG", "DISK I/O", "FANS"} {
		if !strings.Contains(got, want) {
			t.Fatalf("extended grid missing %q:\n%s", want, got)
		}
	}
}

func TestExtendedGridShowsMultipleProcessRows(t *testing.T) {
	// sampleLargeSystemInfo has 8 processes (proc-00..proc-07); the single-host
	// view must show a real list, not just the panel header.
	for _, width := range []int{120, 160} { // 2-col and 3-col layouts
		got := renderMetricsGrid(extendedSampleInfo(), metricHistory{}, width, true)
		if !strings.Contains(got, "proc-00") || !strings.Contains(got, "proc-04") {
			t.Fatalf("width %d: extended process panel missing rows (proc-00/proc-04):\n%s", width, got)
		}
	}
}

func TestIORatePercentIsVisibleAtModestRates(t *testing.T) {
	const ceiling = 125 * 1024 * 1024 // 1 Gbps
	if p := ioRatePercent(0, ceiling); p != 0 {
		t.Fatalf("idle should be 0%%, got %.1f", p)
	}
	// A few MB/s must produce a clearly visible (not ~0) bar — the bug being fixed.
	if p := ioRatePercent(2*1024*1024, ceiling); p < 40 {
		t.Fatalf("2 MB/s should be clearly visible, got %.1f%%", p)
	}
	// Monotonic and capped.
	if ioRatePercent(20*1024*1024, ceiling) <= ioRatePercent(2*1024*1024, ceiling) {
		t.Fatalf("rate scale must be monotonic")
	}
	if p := ioRatePercent(10*ceiling, ceiling); p != 100 {
		t.Fatalf("above ceiling should cap at 100%%, got %.1f", p)
	}
}

func TestFanSectionRendersTrendSparkline(t *testing.T) {
	fans := []internal.FanInfo{{Name: "CPU Fan", RPM: 1200}}
	hist := map[string][]float64{"CPU Fan": {600, 800, 1000, 1200}}
	got := renderFanSection(fans, hist, 60)
	if !strings.Contains(got, "CPU Fan") || !strings.Contains(got, "1200 RPM") {
		t.Fatalf("missing fan row:\n%s", got)
	}
	// renderSparkline draws with ▁▂▃▄▅▆▇█ glyphs.
	if !strings.ContainsAny(got, "▁▂▃▄▅▆▇█") {
		t.Fatalf("fan row missing trend sparkline:\n%s", got)
	}
}

func TestAppendMetricHistoryTracksPerFanRPM(t *testing.T) {
	m := &Model{}
	info := &internal.SystemInfo{Fans: []internal.FanInfo{{Name: "CPU Fan", RPM: 700}}}
	m.appendMetricHistory("h1", info)
	info2 := &internal.SystemInfo{Fans: []internal.FanInfo{{Name: "CPU Fan", RPM: 900}}}
	m.appendMetricHistory("h1", info2)

	got := m.metricHistories["h1"].Fans["CPU Fan"]
	if len(got) != 2 || got[0] != 700 || got[1] != 900 {
		t.Fatalf("per-fan history = %v, want [700 900]", got)
	}
}

func TestDiskIOSectionRendersPerDeviceBar(t *testing.T) {
	diskIO := []internal.DiskIOInfo{
		{Device: "nvme0n1", ReadBps: 80 * 1024 * 1024, WriteBps: 20 * 1024 * 1024},
	}
	got := renderDiskIOSection(diskIO, 60)
	if !strings.Contains(got, "nvme0n1") {
		t.Fatalf("missing device row:\n%s", got)
	}
	// renderThinLineGraph draws with ─/━ glyphs; the row must include a bar.
	if !strings.ContainsAny(got, "─━") {
		t.Fatalf("disk I/O row missing graph bar:\n%s", got)
	}
}

func TestCompactGridOmitsExtraSensors(t *testing.T) {
	got := renderMetricsGrid(extendedSampleInfo(), metricHistory{}, 160, false)
	for _, unwanted := range []string{"SWAP", "LOAD AVG", "DISK I/O", "FANS"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("compact (quad) grid should not contain %q:\n%s", unwanted, got)
		}
	}
}
