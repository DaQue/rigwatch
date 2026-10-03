package gpu

import (
	"regexp"
	"strings"

	"github.com/allisonhere/rigwatch/internal/gpu/base"
)

// PCIProvider is the no-vendor-tool fallback. It finds display adapters from the
// PCI bus and fills in what the kernel exposes, so a GPU is listed even when no
// vendor utility can talk to it. It covers two cases the vendor providers miss:
//
//   - Intel integrated graphics, which have no smi-style tool at all.
//   - NVIDIA cards that nvidia-smi cannot see: legacy-driver and nouveau
//     systems, where the tool is absent or refuses to run.
//
// Intel/legacy-NVIDIA expose no utilization or VRAM counters in sysfs, so those
// read 0; the graphics clock (and temperature, where a hwmon sensor exists) are
// the live signals.
type PCIProvider struct{}

const (
	intelPCIVendor  = "8086"
	nvidiaPCIVendor = "10de"
)

// pciProbeCommand reads, in one round trip, the sysfs attributes worth showing
// for cards this provider covers. The clock paths cover i915 (gt_*), newer i915
// (gt/gt0/rps_*) and xe (tile0/gt0/freq0).
const pciProbeCommand = "sh -c 'grep -H . " +
	"/sys/class/drm/card*/device/vendor " +
	"/sys/class/drm/card*/device/uevent " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/temp1_input " +
	"/sys/class/drm/card*/gt_cur_freq_mhz " +
	"/sys/class/drm/card*/gt_act_freq_mhz " +
	"/sys/class/drm/card*/gt_max_freq_mhz " +
	"/sys/class/drm/card*/gt/gt0/rps_cur_freq_mhz " +
	"/sys/class/drm/card*/gt/gt0/rps_act_freq_mhz " +
	"/sys/class/drm/card*/gt/gt0/rps_max_freq_mhz " +
	"/sys/class/drm/card*/device/tile0/gt0/freq0/cur_freq " +
	"/sys/class/drm/card*/device/tile0/gt0/freq0/max_freq " +
	"2>/dev/null || true'"

// pciDisplayLine matches an lspci -nn line for a display-class device (VGA,
// 3D controller or display controller) and captures slot, description and the
// vendor:device pair.
var pciDisplayLine = regexp.MustCompile(`^(\S+) [^\[]*\[03(?:00|02|80)\]: (.*) \[([0-9a-f]{4}):([0-9a-f]{4})\](?: \(rev [0-9a-f]+\))?$`)

type pciDisplay struct {
	slot   string
	vendor string
	name   string
}

func (p PCIProvider) Name() string { return "pci" }

// Detect is always true: whether there is anything to report is only known after
// reading the bus, and Query does that in a single probe.
func (p PCIProvider) Detect(base.RunCmdFunc) bool { return true }

func (p PCIProvider) Query(runCmd base.RunCmdFunc) ([]base.Device, error) {
	displays := lspciDisplays(runCmd)

	var cards []drmCard
	if out, err := runCmd(pciProbeCommand); err == nil {
		cards = parseDRMCards(out)
	}
	bySlot := map[string]drmCard{}
	for _, c := range cards {
		if c.slot != "" {
			bySlot[c.slot] = c
		}
	}

	// Without lspci (minimal install), fall back to what sysfs alone can say.
	if len(displays) == 0 {
		for _, c := range cards {
			vendor := strings.TrimPrefix(c.attrs["vendor"], "0x")
			if vendor == intelPCIVendor || vendor == nvidiaPCIVendor {
				displays = append(displays, pciDisplay{slot: c.slot, vendor: vendor, name: vendorFallbackName(vendor)})
			}
		}
	}

	// nvidia-smi owns NVIDIA cards whenever it works.
	nvidiaHandled := false
	for _, d := range displays {
		if d.vendor == nvidiaPCIVendor {
			_, err := runCmd("nvidia-smi -L")
			nvidiaHandled = err == nil
			break
		}
	}

	var devices []base.Device
	for _, d := range displays {
		if d.vendor == nvidiaPCIVendor && nvidiaHandled {
			continue
		}
		dev := base.Device{Index: len(devices), Name: d.name, Vendor: vendorLabel(d.vendor)}
		if c, ok := bySlot[d.slot]; ok {
			applyPCICardMetrics(&dev, c)
		}
		devices = append(devices, dev)
	}
	return devices, nil
}

func applyPCICardMetrics(dev *base.Device, c drmCard) {
	dev.Temperature = c.intAttr("temp1_input") / 1000 // m°C → °C
	for _, name := range []string{"gt_act_freq_mhz", "rps_act_freq_mhz", "gt_cur_freq_mhz", "rps_cur_freq_mhz", "cur_freq"} {
		if mhz := c.intAttr(name); mhz > 0 {
			dev.ClockMHz = mhz
			break
		}
	}
	for _, name := range []string{"gt_max_freq_mhz", "rps_max_freq_mhz", "max_freq"} {
		if mhz := c.intAttr(name); mhz > 0 {
			dev.MaxClockMHz = mhz
			break
		}
	}
}

// lspciDisplays lists the Intel and NVIDIA display adapters on the PCI bus.
func lspciDisplays(runCmd base.RunCmdFunc) []pciDisplay {
	out, err := runCmd("lspci -nn")
	if err != nil {
		return nil
	}
	var displays []pciDisplay
	for _, line := range strings.Split(out, "\n") {
		m := pciDisplayLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || (m[3] != intelPCIVendor && m[3] != nvidiaPCIVendor) {
			continue
		}
		displays = append(displays, pciDisplay{slot: m[1], vendor: m[3], name: strings.TrimSpace(m[2])})
	}
	return displays
}

func vendorLabel(vendor string) string {
	if vendor == nvidiaPCIVendor {
		return "nvidia"
	}
	return "intel"
}

func vendorFallbackName(vendor string) string {
	if vendor == nvidiaPCIVendor {
		return "NVIDIA GPU"
	}
	return "Intel integrated graphics"
}
