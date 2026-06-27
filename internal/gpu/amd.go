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
	return false
}

func (p AMDProvider) Query(runCmd base.RunCmdFunc) ([]base.Device, error) {
	if _, err := runCmd("which amd-smi"); err == nil {
		return p.queryModern(runCmd)
	}
	return p.queryLegacy(runCmd)
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
