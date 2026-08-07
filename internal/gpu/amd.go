package gpu

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/allisonhere/rigwatch/internal/gpu/base"
)

type AMDProvider struct{}

func (p AMDProvider) Name() string {
	return "amd"
}

func (p AMDProvider) Detect(runCmd base.RunCmdFunc) bool {
	if _, err := runCmd("which amd-smi"); err == nil {
		return true
	}
	if _, err := runCmd("which rocm-smi"); err == nil {
		return true
	}
	// Neither vendor tool is installed, which is the normal state of a desktop
	// Radeon box: fall back to the kernel's own accounting in sysfs.
	return len(discreteAMDCards(runCmd)) > 0
}

func (p AMDProvider) Query(runCmd base.RunCmdFunc) ([]base.Device, error) {
	if _, err := runCmd("which amd-smi"); err == nil {
		return p.queryModern(runCmd)
	}
	if _, err := runCmd("which rocm-smi"); err == nil {
		return p.queryLegacy(runCmd)
	}
	return p.querySysfs(runCmd)
}

func (p AMDProvider) queryModern(runCmd base.RunCmdFunc) ([]base.Device, error) {
	staticOutput, err := runCmd("amd-smi static --json 2>/dev/null")
	if err != nil {
		return nil, err
	}

	metricsOutput, err := runCmd("amd-smi metric --usage --power --temperature --mem-usage --json 2>/dev/null")
	if err != nil {
		return nil, err
	}

	var staticData struct {
		GPUData []struct {
			GPU  int `json:"gpu"`
			ASIC struct {
				MarketName string `json:"market_name"`
			} `json:"asic"`
			VRAM struct {
				Size struct {
					Value int    `json:"value"`
					Unit  string `json:"unit"`
				} `json:"size"`
			} `json:"vram"`
		} `json:"gpu_data"`
	}

	var metricsData struct {
		GPUData []struct {
			GPU   int `json:"gpu"`
			Usage struct {
				GFXActivity struct {
					Value int `json:"value"`
				} `json:"gfx_activity"`
			} `json:"usage"`
			Power struct {
				SocketPower struct {
					Value int `json:"value"`
				} `json:"socket_power"`
			} `json:"power"`
			Temperature struct {
				Hotspot struct {
					Value int `json:"value"`
				} `json:"hotspot"`
			} `json:"temperature"`
			MemUsage struct {
				TotalVRAM struct {
					Value int `json:"value"`
				} `json:"total_vram"`
				UsedVRAM struct {
					Value int `json:"value"`
				} `json:"used_vram"`
			} `json:"mem_usage"`
		} `json:"gpu_data"`
	}

	if err := json.Unmarshal([]byte(staticOutput), &staticData); err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(metricsOutput), &metricsData); err != nil {
		return nil, err
	}

	var devices []base.Device
	for i, static := range staticData.GPUData {
		if i >= len(metricsData.GPUData) {
			break
		}
		metrics := metricsData.GPUData[i]

		device := base.Device{
			Index:       static.GPU,
			Name:        static.ASIC.MarketName,
			VRAMTotal:   metrics.MemUsage.TotalVRAM.Value,
			VRAMUsed:    metrics.MemUsage.UsedVRAM.Value,
			Utilization: metrics.Usage.GFXActivity.Value,
			PowerDraw:   metrics.Power.SocketPower.Value,
			PowerLimit:  700, // AMD doesn't always report this, conservative estimate
			Temperature: metrics.Temperature.Hotspot.Value,
			Vendor:      "amd",
		}
		devices = append(devices, device)
	}

	return devices, nil
}

func (p AMDProvider) queryLegacy(runCmd base.RunCmdFunc) ([]base.Device, error) {
	output, err := runCmd("rocm-smi --showproductname --showmeminfo vram --showuse -t -P --csv 2>/dev/null")
	if err != nil {
		return nil, err
	}

	var devices []base.Device
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) < 2 {
		return nil, fmt.Errorf("insufficient output from rocm-smi")
	}

	for i, line := range lines[1:] {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			continue
		}

		device := base.Device{
			Index:  i,
			Vendor: "amd",
		}

		if val, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
			device.Temperature = int(val)
		}

		if val, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64); err == nil {
			device.PowerDraw = int(val)
		}

		if val, err := strconv.Atoi(strings.TrimSpace(parts[4])); err == nil {
			device.Utilization = val
		}

		if val, err := strconv.Atoi(strings.TrimSpace(parts[6])); err == nil {
			memOutput, err := runCmd(fmt.Sprintf("rocm-smi -d %d --showmeminfo vram --csv 2>/dev/null | grep -i 'Total VRAM'", i))
			if err == nil {
				memParts := strings.Split(memOutput, ",")
				if len(memParts) >= 2 {
					vramStr := strings.TrimSpace(memParts[1])
					vramStr = strings.TrimSuffix(vramStr, " MB")
					if totalVRAM, err := strconv.Atoi(strings.TrimSpace(vramStr)); err == nil {
						device.VRAMTotal = totalVRAM
						device.VRAMUsed = (totalVRAM * val) / 100
					}
				}
			}
		}

		if len(parts) >= 12 {
			series := strings.TrimSpace(parts[10])
			model := strings.TrimSpace(parts[11])
			if series != "" {
				device.Name = series
				if model != "" && model != series {
					device.Name = fmt.Sprintf("%s (%s)", series, model)
				}
			}
		}

		if device.Name == "" {
			device.Name = "AMD GPU"
		}

		device.PowerLimit = 300 // Conservative estimate for legacy AMD GPUs

		devices = append(devices, device)
	}

	return devices, nil
}

// -- sysfs fallback, for hosts running amdgpu with no vendor tooling installed --

// amdPCIVendor is the PCI vendor ID the kernel reports for AMD/ATI devices.
const amdPCIVendor = "0x1002"

// discreteVRAMFloorMB separates a discrete card from an APU's carve-out. An
// integrated Radeon reports a few hundred MB of stolen system memory; a real
// board reports its soldered VRAM. This is a heuristic, but it beats matching
// on marketing names, which change every generation.
const discreteVRAMFloorMB = 1024

// DRMProbeCommand reads every value the fallback needs in one round trip. Probing
// each attribute separately costs an SSH exec apiece — around thirty per poll on
// a two-card rig, every refresh interval — and this is the same grep -H idiom
// the temperature and fan collectors already use to sweep hwmon.
const DRMProbeCommand = "grep -H . " +
	"/sys/class/drm/card*/device/vendor " +
	"/sys/class/drm/card*/device/uevent " +
	"/sys/class/drm/card*/device/gpu_busy_percent " +
	"/sys/class/drm/card*/device/mem_info_vram_total " +
	"/sys/class/drm/card*/device/mem_info_vram_used " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/temp1_input " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/temp2_input " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/power1_average " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/power1_cap " +
	"2>/dev/null || true"

// drmCardPattern pulls the card number out of a /sys/class/drm path so readings
// from a card's hwmon subdirectory group with the card's own attributes.
var drmCardPattern = regexp.MustCompile(`/card(\d+)/`)

// drmCard is one card's sysfs attributes, keyed by file name (the final path
// element), plus its PCI slot for joining against lspci.
type drmCard struct {
	index int
	slot  string
	attrs map[string]string
}

func (p AMDProvider) querySysfs(runCmd base.RunCmdFunc) ([]base.Device, error) {
	cards := discreteAMDCards(runCmd)
	if len(cards) == 0 {
		return nil, fmt.Errorf("no AMD discrete GPU found via sysfs")
	}

	names := lspciAMDNames(runCmd)

	devices := make([]base.Device, 0, len(cards))
	for i, card := range cards {
		// temp2 is the junction sensor where it exists, and it is the number that
		// matters on a Radeon; temp1 (edge) is the fallback for older boards.
		temp := card.intAttr("temp2_input")
		if temp == 0 {
			temp = card.intAttr("temp1_input")
		}

		name := names[card.slot]
		if name == "" {
			name = "AMD Radeon GPU"
		}

		device := base.Device{
			// Index is the position among the cards actually reported, so it
			// stays dense even when card0 is an APU that was filtered out.
			Index:       i,
			Name:        name,
			VRAMTotal:   card.intAttr("mem_info_vram_total") / 1048576, // bytes → MB
			VRAMUsed:    card.intAttr("mem_info_vram_used") / 1048576,  // bytes → MB
			Utilization: card.intAttr("gpu_busy_percent"),
			PowerDraw:   card.intAttr("power1_average") / 1000000, // µW → W
			PowerLimit:  card.intAttr("power1_cap") / 1000000,     // µW → W
			Temperature: temp / 1000,                              // m°C → °C
			Vendor:      "amd",
		}
		if device.PowerLimit == 0 {
			device.PowerLimit = 700 // amdgpu does not always expose a cap
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// discreteAMDCards returns the AMD cards sysfs reports, lowest card number
// first, excluding integrated graphics.
func discreteAMDCards(runCmd base.RunCmdFunc) []drmCard {
	out, err := runCmd(DRMProbeCommand)
	if err != nil {
		return nil
	}

	byIndex := map[int]*drmCard{}
	for _, line := range strings.Split(out, "\n") {
		path, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		match := drmCardPattern.FindStringSubmatch(path)
		if match == nil {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		card := byIndex[index]
		if card == nil {
			card = &drmCard{index: index, attrs: map[string]string{}}
			byIndex[index] = card
		}

		attr := path[strings.LastIndex(path, "/")+1:]
		value = strings.TrimSpace(value)
		if attr == "uevent" {
			// uevent is many lines; the PCI address is the one worth keeping, and
			// it is what joins this card to its lspci description.
			if slot, found := strings.CutPrefix(value, "PCI_SLOT_NAME="); found {
				// Drop the PCI domain: lspci omits it in its short form.
				card.slot = slot[strings.Index(slot, ":")+1:]
			}
			continue
		}
		card.attrs[attr] = value
	}

	indexes := make([]int, 0, len(byIndex))
	for index := range byIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	cards := make([]drmCard, 0, len(indexes))
	for _, index := range indexes {
		card := byIndex[index]
		if card.attrs["vendor"] != amdPCIVendor {
			continue
		}
		if card.intAttr("mem_info_vram_total")/1048576 < discreteVRAMFloorMB {
			continue
		}
		cards = append(cards, *card)
	}
	return cards
}

// lspciAMDNames maps a PCI slot ("03:00.0") to the device description lspci
// prints for it. Joining on the slot rather than guessing from the name means an
// APU sitting alongside a discrete card cannot steal its label.
func lspciAMDNames(runCmd base.RunCmdFunc) map[string]string {
	out, err := runCmd("lspci -nn")
	if err != nil {
		return nil
	}

	names := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		slot, rest, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		// The bracketed vendor:device pair is exact, unlike a bare "1002" search
		// which also matches bus addresses and revision numbers.
		if !strings.Contains(rest, "["+strings.TrimPrefix(amdPCIVendor, "0x")+":") {
			continue
		}
		if _, description, found := strings.Cut(rest, ": "); found {
			names[slot] = strings.TrimSpace(description)
		}
	}
	return names
}

func (c drmCard) intAttr(name string) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.attrs[name]))
	if err != nil {
		return 0
	}
	return value
}
