package gpu

import (
	"encoding/json"
	"fmt"
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
	// Fallback: check for AMD dGPU via lspci
	out, err := runCmd("lspci -nn")
	if err != nil {
		return false
	}
	// Look for AMD dGPU (not iGPU) with VGA/3D controller
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "1002") && !strings.Contains(strings.ToLower(line), "amd") {
			continue
		}
		if !strings.Contains(line, " VGA ") && !strings.Contains(line, " 3D ") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "granite") || strings.Contains(strings.ToLower(line), "graphics") {
			continue
		}
		return true
	}
	return false
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
			PowerLimit:  700,
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

		device.PowerLimit = 300

		devices = append(devices, device)
	}

	return devices, nil
}

// -- sysfs fallback for systems without amd-smi or rocm-smi --

func (p AMDProvider) querySysfs(runCmd base.RunCmdFunc) ([]base.Device, error) {
	// Find AMD dGPU card by probing drm entries
	var cardPath string
	var hwmonPath string
	for i := 0; i < 8; i++ {
		vendor, _ := runCmd(fmt.Sprintf("cat /sys/class/drm/card%d/device/vendor", i))
		if strings.TrimSpace(vendor) != "0x1002" {
			continue
		}
		// Check for 3+ temp sensors (discrete GPU, not iGPU)
		for j := 0; j < 4; j++ {
			t, err := runCmd(fmt.Sprintf("cat /sys/class/drm/card%d/device/hwmon/hwmon%d/temp3_input", i, j))
			if err == nil && strings.TrimSpace(t) != "" && t != "0" {
				cardPath = fmt.Sprintf("/sys/class/drm/card%d", i)
				hwmonPath = fmt.Sprintf("/sys/class/drm/card%d/device/hwmon/hwmon%d", i, j)
				break
			}
		}
		if cardPath != "" {
			break
		}
	}
	if cardPath == "" {
		return nil, fmt.Errorf("no AMD dGPU found via sysfs")
	}

	dev := sysfsDevice{}

	// Temperature sensors (millidegrees)
	if t, err := runCmd("cat " + hwmonPath + "/temp1_input"); err == nil {
		fmt.Sscanf(strings.TrimSpace(t), "%d", &dev.Temp1)
	}
	if t, err := runCmd("cat " + hwmonPath + "/temp2_input"); err == nil {
		fmt.Sscanf(strings.TrimSpace(t), "%d", &dev.Temp2)
	}
	if t, err := runCmd("cat " + hwmonPath + "/temp3_input"); err == nil {
		fmt.Sscanf(strings.TrimSpace(t), "%d", &dev.Temp3)
	}

	// Power (microwatts)
	if p, err := runCmd("cat " + hwmonPath + "/power1_average"); err == nil {
		fmt.Sscanf(strings.TrimSpace(p), "%d", &dev.Power)
	}
	if p, err := runCmd("cat " + hwmonPath + "/power1_cap"); err == nil {
		fmt.Sscanf(strings.TrimSpace(p), "%d", &dev.PowerCap)
	}

	// Fan
	if f, err := runCmd("cat " + hwmonPath + "/fan1_input"); err == nil {
		fmt.Sscanf(strings.TrimSpace(f), "%d", &dev.Fan)
	}

	// GPU utilization
	if g, err := runCmd("cat " + cardPath + "/device/gpu_busy_percent"); err == nil {
		fmt.Sscanf(strings.TrimSpace(g), "%d", &dev.GPUBusy)
	}

	// VRAM (bytes)
	if v, err := runCmd("cat " + cardPath + "/device/mem_info_vram_total"); err == nil {
		fmt.Sscanf(strings.TrimSpace(v), "%d", &dev.VRAMTot)
		dev.VRAMTot /= 1048576 // bytes → MB
	}
	if v, err := runCmd("cat " + cardPath + "/device/mem_info_vram_used"); err == nil {
		fmt.Sscanf(strings.TrimSpace(v), "%d", &dev.VRAMUsed)
		dev.VRAMUsed /= 1048576 // bytes → MB
	}

	// GPU name from lspci
	nameOut, _ := runCmd("lspci -nn")
	name := ""
	for _, line := range strings.Split(nameOut, "\n") {
		if strings.Contains(line, "1002") && (strings.Contains(line, " VGA ") || strings.Contains(line, " 3D ")) {
			if !strings.Contains(strings.ToLower(line), "granite") {
				if idx := strings.Index(line, ": "); idx >= 0 {
					name = strings.TrimSpace(line[idx+2:])
				}
				break
			}
		}
	}
	if name == "" {
		name = "AMD Radeon GPU"
	}

	// temp values are in millidegrees Celsius
	temp := dev.Temp2 // junction temp
	if temp == 0 {
		temp = dev.Temp1
	}

	device := base.Device{
		Index:       0,
		Name:        name,
		VRAMTotal:   dev.VRAMTot,
		VRAMUsed:    dev.VRAMUsed,
		Utilization: dev.GPUBusy,
		PowerDraw:   dev.Power / 1000000,  // microwatts → watts
		PowerLimit:  dev.PowerCap / 1000000,
		Temperature: temp / 1000, // millidegrees → Celsius
		Vendor:      "amd",
	}
	if device.PowerLimit == 0 {
		device.PowerLimit = 700
	}
	return []base.Device{device}, nil
}

type sysfsDevice struct {
	Name     string
	Temp1    int
	Temp2    int
	Temp3    int
	Power    int
	PowerCap int
	Fan      int
	GPUBusy  int
	VRAMTot  int
	VRAMUsed int
}
