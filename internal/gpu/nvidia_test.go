package gpu

import (
	"fmt"
	"strings"
	"testing"
)

func TestNvidiaQueryKeepsCoreMetricsWhenOptionalMetricsFail(t *testing.T) {
	provider := NvidiaProvider{}
	devices, err := provider.Query(func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "index,name,memory.total,memory.used,utilization.gpu"):
			return "0, RTX Test, 24576, 12288, 65\n", nil
		case strings.Contains(cmd, "power.draw"):
			return "", fmt.Errorf("optional metrics unsupported")
		default:
			return "", fmt.Errorf("unexpected command: %s", cmd)
		}
	})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices len = %d, want 1", len(devices))
	}
	got := devices[0]
	if got.VRAMTotal != 24576 || got.VRAMUsed != 12288 || got.Utilization != 65 {
		t.Fatalf("core metrics = %+v, want VRAM/util preserved", got)
	}
	if got.PowerDraw != 0 || got.Temperature != 0 {
		t.Fatalf("optional metrics should remain zero on optional failure: %+v", got)
	}
}

func TestNvidiaQueryMergesOptionalMetrics(t *testing.T) {
	provider := NvidiaProvider{}
	devices, err := provider.Query(func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "index,name,memory.total,memory.used,utilization.gpu"):
			return "0, RTX Test, 24576, 12288, 65\n", nil
		case strings.Contains(cmd, "index,power.draw,power.limit,temperature.gpu"):
			return "0, 250.5, 350.0, 70\n", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", cmd)
		}
	})
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices len = %d, want 1", len(devices))
	}
	got := devices[0]
	if got.PowerDraw != 250 || got.PowerLimit != 350 || got.Temperature != 70 {
		t.Fatalf("optional metrics = %+v, want power/temp merged", got)
	}
}

func TestNvidiaThrottleReasonsDecodesBits(t *testing.T) {
	got := nvidiaThrottleReasons("0x0000000000000064") // 0x4 | 0x20 | 0x40
	want := []string{"power cap", "thermal slowdown", "hw thermal slowdown"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if r := nvidiaThrottleReasons("0x0000000000000001"); len(r) != 0 { // idle is not a problem
		t.Fatalf("idle flagged: %v", r)
	}
	if r := nvidiaThrottleReasons("[N/A]"); r != nil {
		t.Fatalf("unparseable gave %v", r)
	}
}

func TestParseNvidiaPmonReadsColumnsByName(t *testing.T) {
	out := `# gpu         pid   type     fb   ccpm     sm    mem    enc    dec    jpg    ofa    command
# Idx           #    C/G     MB     MB      %      %      %      %      %      %    name
    0       4242     C+G   8123      0     87     30      -      -      -      -    python3
    1       9001       G    120      0      -      -      -      -      -      -    Xorg
`
	procs := parseNvidiaPmon(out)
	if len(procs) != 2 {
		t.Fatalf("got %d, want 2: %+v", len(procs), procs)
	}
	if procs[0].PID != 4242 || procs[0].VRAMMB != 8123 || procs[0].UtilPct != 87 || procs[0].Name != "python3" || procs[0].GPU != "0" {
		t.Errorf("proc 0 = %+v", procs[0])
	}
	if procs[1].UtilPct != -1 || procs[1].GPU != "1" {
		t.Errorf("proc 1 = %+v, want unknown utilization on GPU 1", procs[1])
	}
}
