package gpu

import (
	"errors"
	"strings"
	"testing"

	"github.com/allisonhere/rigwatch/internal/gpu/base"
)

const oldLaptopLspci = `00:00.0 Host bridge [0600]: Intel Corporation 2nd Generation Core Processor Family DRAM Controller [8086:0104] (rev 09)
00:02.0 VGA compatible controller [0300]: Intel Corporation 2nd Generation Core Processor Family Integrated Graphics Controller [8086:0116] (rev 09)
01:00.0 VGA compatible controller [0300]: NVIDIA Corporation GF108M [GeForce GT 540M] [10de:0df4] (rev a1)
02:00.0 Ethernet controller [0200]: Realtek Semiconductor Co., Ltd. RTL8111/8168 [10ec:8168] (rev 06)`

const intelProbe = `/sys/class/drm/card0/device/vendor:0x8086
/sys/class/drm/card0/device/uevent:PCI_SLOT_NAME=0000:00:02.0
/sys/class/drm/card0/gt_cur_freq_mhz:350
/sys/class/drm/card0/gt_act_freq_mhz:349
/sys/class/drm/card0/gt_max_freq_mhz:1100`

func fakeRun(lspci, probe string, smiOK bool) base.RunCmdFunc {
	return func(cmd string) (string, error) {
		switch cmd {
		case "lspci -nn":
			if lspci == "" {
				return "", errors.New("not found")
			}
			return lspci, nil
		case pciProbeCommand:
			return probe, nil
		case "nvidia-smi -L":
			if smiOK {
				return "GPU 0: GeForce", nil
			}
			return "", errors.New("NVIDIA-SMI has failed")
		}
		return "", errors.New("unexpected: " + cmd)
	}
}

func TestPCIProviderListsIntelAndLegacyNvidia(t *testing.T) {
	devices, err := PCIProvider{}.Query(fakeRun(oldLaptopLspci, intelProbe, false))
	if err != nil || len(devices) != 2 {
		t.Fatalf("got %d devices, err %v: %+v", len(devices), err, devices)
	}
	intel, nv := devices[0], devices[1]
	if intel.Vendor != "intel" || !strings.Contains(intel.Name, "Integrated Graphics") {
		t.Fatalf("intel device wrong: %+v", intel)
	}
	if intel.ClockMHz != 349 || intel.MaxClockMHz != 1100 {
		t.Fatalf("intel clocks = %d/%d, want 349/1100", intel.ClockMHz, intel.MaxClockMHz)
	}
	if nv.Vendor != "nvidia" || !strings.Contains(nv.Name, "GeForce GT 540M") || strings.Contains(nv.Name, "10de") {
		t.Fatalf("nvidia device wrong: %+v", nv)
	}
	if intel.Index != 0 || nv.Index != 1 {
		t.Fatalf("indexes not dense: %d, %d", intel.Index, nv.Index)
	}
}

func TestPCIProviderLeavesNvidiaToSmiWhenItWorks(t *testing.T) {
	devices, _ := PCIProvider{}.Query(fakeRun(oldLaptopLspci, intelProbe, true))
	if len(devices) != 1 || devices[0].Vendor != "intel" {
		t.Fatalf("want only the intel iGPU, got %+v", devices)
	}
}

func TestPCIProviderWithoutLspciUsesSysfs(t *testing.T) {
	devices, _ := PCIProvider{}.Query(fakeRun("", intelProbe, false))
	if len(devices) != 1 || devices[0].Name != "Intel integrated graphics" || devices[0].ClockMHz != 349 {
		t.Fatalf("got %+v", devices)
	}
}

func TestPCIProviderIgnoresAMDAndNonDisplayDevices(t *testing.T) {
	lspci := `c4:00.0 Display controller [0380]: Advanced Micro Devices, Inc. [AMD/ATI] Strix Halo [1002:1586] (rev c1)
00:1f.3 Audio device [0403]: Intel Corporation HD Audio [8086:1c20]`
	devices, _ := PCIProvider{}.Query(fakeRun(lspci, "", false))
	if len(devices) != 0 {
		t.Fatalf("expected nothing, got %+v", devices)
	}
}
