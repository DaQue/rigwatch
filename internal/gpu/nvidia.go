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
	mergeNvidiaClocks(runCmd, devices)
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

// nvidiaClockQueries are tried in order. The throttle-reason field was renamed
// from clocks_throttle_reasons to clocks_event_reasons in newer drivers, and an
// unknown field fails the whole query, so each spelling gets its own attempt and
// the last one still yields clocks without reasons.
var nvidiaClockQueries = []struct {
	fields  string
	reasons bool
}{
	{"index,clocks.sm,clocks.max.sm,clocks_event_reasons.active", true},
	{"index,clocks.sm,clocks.max.sm,clocks_throttle_reasons.active", true},
	{"index,clocks.sm,clocks.max.sm", false},
}

func mergeNvidiaClocks(runCmd base.RunCmdFunc, devices []base.Device) {
	byIndex := make(map[int]*base.Device, len(devices))
	for i := range devices {
		byIndex[devices[i].Index] = &devices[i]
	}
	for _, q := range nvidiaClockQueries {
		output, err := runCmd("nvidia-smi --query-gpu=" + q.fields + " --format=csv,noheader,nounits")
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			parts := strings.Split(line, ",")
			if len(parts) < 3 {
				continue
			}
			idx, ok := parseNvidiaInt(parts[0])
			if !ok || byIndex[idx] == nil {
				continue
			}
			dev := byIndex[idx]
			dev.ClockMHz, _ = parseNvidiaInt(parts[1])
			dev.MaxClockMHz, _ = parseNvidiaInt(parts[2])
			if q.reasons && len(parts) >= 4 {
				dev.Throttle = nvidiaThrottleReasons(parts[3])
			}
		}
		return
	}
}

// NVML clocks-event-reason bits worth surfacing. Idle (0x1), application clock
// settings (0x2) and sync boost (0x10) describe configuration, not a problem.
var nvidiaThrottleBits = []struct {
	bit  uint64
	name string
}{
	{0x4, "power cap"},
	{0x8, "hw slowdown"},
	{0x20, "thermal slowdown"},
	{0x40, "hw thermal slowdown"},
	{0x80, "power brake"},
}

func nvidiaThrottleReasons(field string) []string {
	field = strings.TrimSpace(field)
	field = strings.TrimPrefix(strings.TrimPrefix(field, "0x"), "0X")
	mask, err := strconv.ParseUint(field, 16, 64)
	if err != nil {
		return nil
	}
	var reasons []string
	for _, r := range nvidiaThrottleBits {
		if mask&r.bit != 0 {
			reasons = append(reasons, r.name)
		}
	}
	return reasons
}

// Processes reports GPU processes via nvidia-smi pmon, which covers graphics as
// well as compute clients and gives per-process SM utilization. If pmon is not
// supported (older cards, vGPU) it falls back to the compute-app list, which has
// memory but no utilization.
func (p NvidiaProvider) Processes(runCmd base.RunCmdFunc) []base.Process {
	if out, err := runCmd("nvidia-smi pmon -c 1 -s um"); err == nil {
		if procs := parseNvidiaPmon(out); len(procs) > 0 {
			return procs
		}
	}
	out, err := runCmd("nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits")
	if err != nil {
		return nil
	}
	var procs []base.Process
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		pid, ok := parseNvidiaInt(parts[0])
		if !ok {
			continue
		}
		mem, _ := parseNvidiaInt(parts[2])
		name := strings.TrimSpace(parts[1])
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		procs = append(procs, base.Process{PID: pid, Name: name, VRAMMB: mem, UtilPct: -1})
	}
	return procs
}

// parseNvidiaPmon reads `nvidia-smi pmon` by column name; the header line is
// "# gpu pid type fb ccpm sm mem enc dec jpg ofa command" and varies by driver.
func parseNvidiaPmon(output string) []base.Process {
	var header []string
	col := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		return -1
	}
	var procs []base.Process
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if header == nil { // first comment line names the columns; the next is units
				header = strings.Fields(strings.TrimPrefix(line, "#"))
			}
			continue
		}
		if header == nil {
			continue
		}
		fields := strings.Fields(line)
		pidCol, cmdCol := col("pid"), col("command")
		if pidCol < 0 || cmdCol < 0 || len(fields) <= cmdCol {
			continue
		}
		pid, err := strconv.Atoi(fields[pidCol])
		if err != nil {
			continue
		}
		proc := base.Process{PID: pid, Name: strings.Join(fields[cmdCol:], " "), UtilPct: -1}
		if i := col("gpu"); i >= 0 && i < len(fields) {
			proc.GPU = fields[i]
		}
		if i := col("fb"); i >= 0 && i < len(fields) {
			proc.VRAMMB, _ = strconv.Atoi(fields[i])
		}
		if i := col("sm"); i >= 0 && i < len(fields) {
			if v, err := strconv.Atoi(fields[i]); err == nil {
				proc.UtilPct = v
			}
		}
		procs = append(procs, proc)
	}
	return procs
}
