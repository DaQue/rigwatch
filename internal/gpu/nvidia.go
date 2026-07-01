package gpu

import (
	"strconv"
	"strings"

	"github.com/allisonhere/rigwatch/internal/gpu/base"
)

type NvidiaProvider struct{}

func (p NvidiaProvider) Name() string {
	return "nvidia"
}

func (p NvidiaProvider) Detect(runCmd base.RunCmdFunc) bool {
	_, err := runCmd("which nvidia-smi")
	return err == nil
}

func (p NvidiaProvider) Query(runCmd base.RunCmdFunc) ([]base.Device, error) {
	output, err := runCmd("nvidia-smi --query-gpu=index,name,memory.total,memory.used,utilization.gpu --format=csv,noheader,nounits")
	if err != nil {
		return nil, err
	}

	var devices []base.Device
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			device := base.Device{
				Vendor: "nvidia",
			}

			if val, ok := parseNvidiaInt(parts[0]); ok {
				device.Index = val
			}
			device.Name = strings.TrimSpace(parts[1])

			if val, ok := parseNvidiaInt(parts[2]); ok {
				device.VRAMTotal = val
			}
			if val, ok := parseNvidiaInt(parts[3]); ok {
				device.VRAMUsed = val
			}
			if val, ok := parseNvidiaInt(parts[4]); ok {
				device.Utilization = val
			}

			devices = append(devices, device)
		}
	}

	mergeNvidiaOptionalMetrics(runCmd, devices)
	return devices, nil
}

func mergeNvidiaOptionalMetrics(runCmd base.RunCmdFunc, devices []base.Device) {
	if len(devices) == 0 {
		return
	}

	output, err := runCmd("nvidia-smi --query-gpu=index,power.draw,power.limit,temperature.gpu --format=csv,noheader,nounits")
	if err != nil {
		return
	}

	byIndex := make(map[int]*base.Device, len(devices))
	for i := range devices {
		byIndex[devices[i].Index] = &devices[i]
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 4 {
			continue
		}
		idx, ok := parseNvidiaInt(parts[0])
		if !ok {
			continue
		}
		device := byIndex[idx]
		if device == nil {
			continue
		}
		if val, ok := parseNvidiaFloatAsInt(parts[1]); ok {
			device.PowerDraw = val
		}
		if val, ok := parseNvidiaFloatAsInt(parts[2]); ok {
			device.PowerLimit = val
		}
		if val, ok := parseNvidiaInt(parts[3]); ok {
			device.Temperature = val
		}
	}
}

func parseNvidiaInt(field string) (int, bool) {
	val, err := strconv.Atoi(strings.TrimSpace(field))
	return val, err == nil
}

func parseNvidiaFloatAsInt(field string) (int, bool) {
	val, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
	return int(val), err == nil
}
