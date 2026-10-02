package gpu

import (
	"fmt"
	"strings"
	"testing"
)

// drmProbeFixture is a realistic grep -H . sweep of /sys/class/drm on a machine
// with an APU (card0) and a discrete Radeon (card1), which is the layout the
// sysfs fallback most has to get right.
const drmProbeFixture = `/sys/class/drm/card0/device/vendor:0x1002
/sys/class/drm/card0/device/uevent:DRIVER=amdgpu
/sys/class/drm/card0/device/uevent:PCI_SLOT_NAME=0000:16:00.0
/sys/class/drm/card0/device/gpu_busy_percent:0
/sys/class/drm/card0/device/mem_info_vram_total:536870912
/sys/class/drm/card0/device/mem_info_vram_used:8388608
/sys/class/drm/card0/device/hwmon/hwmon1/temp1_input:41000
/sys/class/drm/card1/device/vendor:0x1002
/sys/class/drm/card1/device/uevent:DRIVER=amdgpu
/sys/class/drm/card1/device/uevent:PCI_SLOT_NAME=0000:03:00.0
/sys/class/drm/card1/device/gpu_busy_percent:73
/sys/class/drm/card1/device/mem_info_vram_total:17163091968
/sys/class/drm/card1/device/mem_info_vram_used:4294967296
/sys/class/drm/card1/device/hwmon/hwmon2/temp1_input:58000
/sys/class/drm/card1/device/hwmon/hwmon2/temp2_input:71000
/sys/class/drm/card1/device/hwmon/hwmon2/power1_average:214000000
/sys/class/drm/card1/device/hwmon/hwmon2/power1_cap:317000000
`

const lspciFixture = `00:14.3 Network controller [0280]: Intel Corporation Wi-Fi 6 AX200 [8086:2723] (rev 1a)
03:00.0 VGA compatible controller [0300]: Advanced Micro Devices, Inc. [AMD/ATI] Navi 48 [Radeon RX 9070 XT] [1002:7550] (rev c0)
03:00.1 Audio device [0403]: Advanced Micro Devices, Inc. [AMD/ATI] Navi 48 HDMI Audio [1002:ab40]
16:00.0 VGA compatible controller [0300]: Advanced Micro Devices, Inc. [AMD/ATI] Granite Ridge [Radeon Graphics] [1002:13c0] (rev c1)
`

// sysfsOnlyHost answers like a box with amdgpu loaded but no vendor tooling.
func sysfsOnlyHost(probe, lspci string) func(string) (string, error) {
	return func(cmd string) (string, error) {
		switch {
		case strings.HasPrefix(cmd, "which "):
			return "", fmt.Errorf("not installed")
		case cmd == DRMProbeCommand:
			return probe, nil
		case strings.HasPrefix(cmd, "lspci"):
			return lspci, nil
		default:
			return "", fmt.Errorf("unexpected command: %s", cmd)
		}
	}
}

func TestAMDSysfsReportsDiscreteCardOnly(t *testing.T) {
	devices, err := AMDProvider{}.Query(sysfsOnlyHost(drmProbeFixture, lspciFixture))
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	// card0 is the APU's 512 MB carve-out and must not be reported as a GPU.
	if len(devices) != 1 {
		t.Fatalf("devices len = %d, want 1 (discrete card only): %+v", len(devices), devices)
	}

	got := devices[0]
	if got.Index != 0 {
		t.Errorf("Index = %d, want 0 — indexes are dense over reported cards", got.Index)
	}
	// Joined to lspci by PCI slot, so the APU's description cannot leak across.
	if !strings.Contains(got.Name, "Radeon RX 9070 XT") {
		t.Errorf("Name = %q, want the discrete card's lspci description", got.Name)
	}
	if got.VRAMTotal != 16368 || got.VRAMUsed != 4096 {
		t.Errorf("VRAM = %d/%d MB, want 16368/4096", got.VRAMUsed, got.VRAMTotal)
	}
	if got.Utilization != 73 {
		t.Errorf("Utilization = %d, want 73", got.Utilization)
	}
	if got.PowerDraw != 214 || got.PowerLimit != 317 {
		t.Errorf("power = %d/%d W, want 214/317", got.PowerDraw, got.PowerLimit)
	}
	// temp2 is the junction sensor and wins over temp1's edge reading.
	if got.Temperature != 71 {
		t.Errorf("Temperature = %d, want 71 (junction)", got.Temperature)
	}
	if got.Vendor != "amd" {
		t.Errorf("Vendor = %q, want amd", got.Vendor)
	}
}

func TestAMDSysfsReportsEveryDiscreteCard(t *testing.T) {
	// Two discrete cards, neither with a junction sensor: both must appear, with
	// dense indexes and the edge temperature as the fallback.
	probe := `/sys/class/drm/card0/device/vendor:0x1002
/sys/class/drm/card0/device/uevent:PCI_SLOT_NAME=0000:03:00.0
/sys/class/drm/card0/device/mem_info_vram_total:8589934592
/sys/class/drm/card0/device/hwmon/hwmon0/temp1_input:52000
/sys/class/drm/card1/device/vendor:0x1002
/sys/class/drm/card1/device/uevent:PCI_SLOT_NAME=0000:0a:00.0
/sys/class/drm/card1/device/mem_info_vram_total:8589934592
/sys/class/drm/card1/device/hwmon/hwmon1/temp1_input:49000
`
	devices, err := AMDProvider{}.Query(sysfsOnlyHost(probe, ""))
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices len = %d, want 2", len(devices))
	}
	if devices[0].Index != 0 || devices[1].Index != 1 {
		t.Errorf("indexes = %d,%d, want 0,1", devices[0].Index, devices[1].Index)
	}
	if devices[0].Temperature != 52 || devices[1].Temperature != 49 {
		t.Errorf("temps = %d,%d, want 52,49 (edge sensor fallback)",
			devices[0].Temperature, devices[1].Temperature)
	}
	// With no lspci match the label still has to be something readable.
	if devices[0].Name != "AMD Radeon GPU" {
		t.Errorf("Name = %q, want the generic fallback", devices[0].Name)
	}
	// amdgpu did not expose a cap here; report unknown (0) rather than invent one.
	if devices[0].PowerLimit != 0 {
		t.Errorf("PowerLimit = %d, want 0 (unknown)", devices[0].PowerLimit)
	}
}

func TestAMDSysfsIgnoresNonAMDAndIntegratedCards(t *testing.T) {
	probe := `/sys/class/drm/card0/device/vendor:0x10de
/sys/class/drm/card0/device/mem_info_vram_total:25769803776
/sys/class/drm/card1/device/vendor:0x1002
/sys/class/drm/card1/device/mem_info_vram_total:268435456
`
	if _, err := (AMDProvider{}).Query(sysfsOnlyHost(probe, "")); err == nil {
		t.Fatal("expected an error when only an NVIDIA card and an APU are present")
	}
	if (AMDProvider{}).Detect(sysfsOnlyHost(probe, "")) {
		t.Fatal("Detect reported an AMD GPU with only an NVIDIA card and an APU present")
	}
}

func TestAMDDetectFindsSysfsCardWithoutVendorTooling(t *testing.T) {
	if !(AMDProvider{}).Detect(sysfsOnlyHost(drmProbeFixture, lspciFixture)) {
		t.Fatal("Detect missed a discrete Radeon reported by sysfs")
	}
}

func TestAMDSysfsSurvivesAnEmptyProbe(t *testing.T) {
	// grep finds nothing on a host with no DRM devices; the trailing `|| true`
	// means that arrives as success with empty output, not as an error.
	if _, err := (AMDProvider{}).Query(sysfsOnlyHost("", "")); err == nil {
		t.Fatal("expected an error when sysfs reports no cards")
	}
}

func TestAMDVendorToolingStillWins(t *testing.T) {
	// rocm-smi present: the sysfs path must not be taken.
	probed := false
	_, _ = AMDProvider{}.Query(func(cmd string) (string, error) {
		switch {
		case cmd == "which rocm-smi":
			return "/usr/bin/rocm-smi", nil
		case cmd == DRMProbeCommand:
			probed = true
			return "", nil
		case strings.HasPrefix(cmd, "rocm-smi"):
			return "device,temp,x,power,use,y,mem\ncard0,60,,150,42,,50\n", nil
		default:
			return "", fmt.Errorf("not installed")
		}
	})
	if probed {
		t.Fatal("sysfs probed even though rocm-smi is installed")
	}
}

// Captured from a Strix Halo box: a warning line precedes the table, and the
// columns do not sit where older ROCm releases put them.
func TestParseRocmSmiCSVReadsColumnsByName(t *testing.T) {
	out := `WARNING: AMD GPU device(s) is/are in a low-power state. Check power control/runtime_status

device,Temperature (Sensor edge) (C),Current Socket Graphics Package Power (W),GPU use (%),VRAM Total Memory (B),VRAM Total Used Memory (B),Card Series,Card Model,Card Vendor,Card SKU,Subsystem ID,Device Rev,Node ID,GUID,GFX Version
card0,54.0,20.358,2,536870912,511442944,AMD Radeon 8060S Graphics,0x1586,Advanced Micro Devices Inc. [AMD/ATI],STRXLGEN,0x1fb3,0xc1,1,64042,gfx1151
card1,40.0,100.0,97,17163091968,8581545984,AMD Radeon RX 9070 XT,0x7550,Advanced Micro Devices Inc. [AMD/ATI],X,0x1,0xc0,2,1,gfx1201
`
	devices := parseRocmSmiCSV(out)
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2: %+v", len(devices), devices)
	}
	d := devices[0]
	if d.Name != "AMD Radeon 8060S Graphics" || d.Utilization != 2 || d.Temperature != 54 || d.PowerDraw != 20 ||
		d.VRAMTotal != 512 || d.VRAMUsed != 487 || d.PowerLimit != 0 {
		t.Errorf("card0 = %+v", d)
	}
	if devices[1].Index != 1 || devices[1].Utilization != 97 || devices[1].VRAMTotal != 16368 || devices[1].VRAMUsed != 8184 {
		t.Errorf("card1 = %+v", devices[1])
	}
	if parseRocmSmiCSV("WARNING only") != nil {
		t.Error("expected nil without a table")
	}
}
