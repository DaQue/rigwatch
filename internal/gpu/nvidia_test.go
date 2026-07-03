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
