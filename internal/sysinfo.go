package internal

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/allisonhere/rigwatch/internal/gpu"
)

type SystemInfo struct {
	CPU       CPUInfo
	GPUs      []GPUInfo
	RAM       RAMInfo
	Disk      []DiskInfo
	Temps     []TemperatureInfo
	Network   []NetworkInfo
	Processes []ProcessInfo
}

type CPUInfo struct {
	Model        string
	Count        string
	Usage        string
	UsagePercent float64
	Cores        []CPUCoreInfo
}

type CPUCoreInfo struct {
	Index        int
	UsagePercent float64
}

type GPUInfo struct {
	Index       string
	Name        string
	VRAMTotal   int // in MB
	VRAMUsed    int // in MB
	Utilization int // percentage
	PowerDraw   int // in Watts
	PowerLimit  int // in Watts
	Temperature int // in Celsius
}

type RAMInfo struct {
	Total        int // in MB
	Used         int // in MB
	UsagePercent float64
}

type DiskInfo struct {
	Device       string
	Size         string
	Used         string
	Available    string
	UsagePercent string
	MountPoint   string
}

type TemperatureInfo struct {
	Name    string
	Celsius float64
}

type NetworkInfo struct {
	Name    string
	RXBytes uint64
	TXBytes uint64
	RXBps   uint64
	TXBps   uint64
}

type ProcessInfo struct {
	PID        int
	Command    string
	CPUPercent float64
	MemPercent float64
}

func GatherSystemInfo(client *SSHClient) (*SystemInfo, error) {
	info := &SystemInfo{}

	cpuInfo, err := getCPUInfo(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}
	info.CPU = cpuInfo

	gpuInfo, _ := getGPUInfo(client)
	info.GPUs = gpuInfo

	ramInfo, err := getRAMInfo(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get RAM info: %w", err)
	}
	info.RAM = ramInfo

	diskInfo, err := getDiskInfo(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk info: %w", err)
	}
	info.Disk = diskInfo

	if temps, err := getTemperatureInfo(client); err == nil {
		info.Temps = temps
	}

	if networkInfo, err := getNetworkInfo(client); err == nil {
		info.Network = networkInfo
	}

	if processInfo, err := getProcessInfo(client); err == nil {
		info.Processes = processInfo
	}

	return info, nil
}

func getCPUInfo(client *SSHClient) (CPUInfo, error) {
	info := CPUInfo{}

	output, err := client.ExecuteCommand("lscpu | grep -E 'Model name|CPU\\(s\\):'")
	if err == nil {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Model name:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.Model = strings.TrimSpace(parts[1])
				}
			} else if strings.HasPrefix(line, "CPU(s):") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					info.Count = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	output, err = client.ExecuteCommand("top -bn1 -1 | grep -E '^(%Cpu|CPU:)'")
	if err == nil {
		usage, cores := parseTopCPUUsage(output)
		if usage > 0 || len(cores) > 0 {
			info.UsagePercent = usage
			info.Usage = fmt.Sprintf("%.1f%%", usage)
			info.Cores = normalizeCPUCores(info.Count, cores)
		}
	}

	if info.Usage == "" {
		info.Usage = "N/A"
	}

	return info, nil
}

func parseTopCPUUsage(output string) (float64, []CPUCoreInfo) {
	var aggregate float64
	var cores []CPUCoreInfo

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		usage, ok := parseCPUUsageLine(line)
		if !ok {
			continue
		}

		if strings.HasPrefix(line, "%Cpu(s)") || strings.HasPrefix(line, "CPU:") {
			aggregate = usage
			continue
		}

		if strings.HasPrefix(line, "%Cpu") {
			label := strings.TrimPrefix(strings.Fields(line)[0], "%Cpu")
			label = strings.TrimSuffix(label, ":")
			if idx, err := strconv.Atoi(label); err == nil {
				cores = append(cores, CPUCoreInfo{Index: idx, UsagePercent: usage})
			}
		}
	}

	if aggregate == 0 && len(cores) > 0 {
		var total float64
		for _, core := range cores {
			total += core.UsagePercent
		}
		aggregate = total / float64(len(cores))
	}

	return aggregate, cores
}

func normalizeCPUCores(count string, cores []CPUCoreInfo) []CPUCoreInfo {
	if len(cores) == 0 {
		return cores
	}

	sort.Slice(cores, func(i, j int) bool {
		return cores[i].Index < cores[j].Index
	})

	coreCount, err := strconv.Atoi(strings.TrimSpace(count))
	if err != nil || coreCount <= 0 {
		return cores
	}

	normalized := make([]CPUCoreInfo, coreCount)
	for i := range normalized {
		normalized[i] = CPUCoreInfo{Index: i}
	}

	for _, core := range cores {
		if core.Index >= 0 && core.Index < coreCount {
			normalized[core.Index] = core
		}
	}

	return normalized
}

func parseCPUUsageLine(line string) (float64, bool) {
	fields := strings.Fields(strings.ReplaceAll(line, ",", ""))
	for i, field := range fields {
		label := strings.TrimSuffix(field, ":")
		if label != "id" && label != "idle" {
			continue
		}

		if i == 0 {
			return 0, false
		}

		value := strings.TrimSuffix(fields[i-1], "%")
		idle, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, false
		}
		return 100 - idle, true
	}

	return 0, false
}

func getGPUInfo(client *SSHClient) ([]GPUInfo, error) {
	runCmd := func(cmd string) (string, error) {
		return client.ExecuteCommand(cmd)
	}

	devices, err := gpu.QueryAll(runCmd)
	if err != nil {
		return []GPUInfo{}, nil
	}

	gpus := make([]GPUInfo, len(devices))
	for i, dev := range devices {
		gpus[i] = GPUInfo{
			Index:       fmt.Sprintf("%d", dev.Index),
			Name:        dev.Name,
			VRAMTotal:   dev.VRAMTotal,
			VRAMUsed:    dev.VRAMUsed,
			Utilization: dev.Utilization,
			PowerDraw:   dev.PowerDraw,
			PowerLimit:  dev.PowerLimit,
			Temperature: dev.Temperature,
		}
	}

	return gpus, nil
}

func getRAMInfo(client *SSHClient) (RAMInfo, error) {
	info := RAMInfo{}

	output, err := client.ExecuteCommand("free -m | grep Mem:")
	if err != nil {
		return info, err
	}

	parts := strings.Fields(output)
	if len(parts) >= 3 {
		if val, err := strconv.Atoi(parts[1]); err == nil {
			info.Total = val
		}
		if val, err := strconv.Atoi(parts[2]); err == nil {
			info.Used = val
		}

		if info.Total > 0 {
			info.UsagePercent = (float64(info.Used) / float64(info.Total)) * 100
		}
	}

	return info, nil
}

func getDiskInfo(client *SSHClient) ([]DiskInfo, error) {
	output, err := client.ExecuteCommand("df -h | grep -E '^/dev/'")
	if err != nil {
		return nil, err
	}

	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 6 {
			disk := DiskInfo{
				Device:       parts[0],
				Size:         parts[1],
				Used:         parts[2],
				Available:    parts[3],
				UsagePercent: parts[4],
				MountPoint:   parts[5],
			}
			disks = append(disks, disk)
		}
	}

	return disks, nil
}

func getTemperatureInfo(client *SSHClient) ([]TemperatureInfo, error) {
	output, err := client.ExecuteCommand("grep -H . /sys/class/thermal/thermal_zone*/type /sys/class/thermal/thermal_zone*/temp")
	if err != nil {
		// Try hwmon fallback for CPU temp
		return getHwmonCPUTemp(client)
	}
	temps := parseThermalZones(output)
	if len(temps) == 0 {
		return getHwmonCPUTemp(client)
	}
	return temps, nil
}

func getHwmonCPUTemp(client *SSHClient) ([]TemperatureInfo, error) {
	output, err := client.ExecuteCommand("grep -H . /sys/class/hwmon/hwmon*/temp*_label /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null || true")
	if err != nil {
		return nil, err
	}
	if output == "" {
		return nil, fmt.Errorf("no hwmon temperature sensors")
	}
	temps := parseHwmonTemps(output)
	if len(temps) == 0 {
		return nil, fmt.Errorf("no CPU temperature in hwmon")
	}
	return temps, nil
}

func parseHwmonTemps(output string) []TemperatureInfo {
	type sensorData struct {
		label   string
		celsius float64
		hasTemp bool
	}
	sensors := make(map[string]sensorData)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		path, value := parts[0], strings.TrimSpace(parts[1])

		// Extract sensor key: /sys/class/hwmon/hwmon0/temp1_label -> hwmon0/temp1
		relPath := strings.TrimPrefix(path, "/sys/class/hwmon/")
		lastSlash := strings.LastIndex(relPath, "/")
		if lastSlash < 0 {
			continue
		}
		dir := relPath[:lastSlash]
		file := relPath[lastSlash+1:]
		if !strings.HasPrefix(file, "temp") {
			continue
		}
		sensorPart := file
		if idx := strings.IndexAny(sensorPart, "_."); idx >= 0 {
			sensorPart = sensorPart[:idx]
		}
		key := dir + "/" + sensorPart

		data := sensors[key]
		if strings.HasSuffix(path, "_label") {
			data.label = value
		} else if strings.HasSuffix(path, "_input") {
			raw, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			if raw > 1000 {
				raw = raw / 1000
			}
			data.celsius = raw
			data.hasTemp = true
		}
		sensors[key] = data
	}

	cpuLabels := map[string]bool{
		"package id 0": true, "core 0": true, "tctl": true,
		"tccd1": true, "cpu": true, "physical id 0": true,
	}
	var temps []TemperatureInfo
	for _, data := range sensors {
		if !data.hasTemp {
			continue
		}
		label := strings.ToLower(data.label)
		if cpuLabels[label] || strings.Contains(label, "package") || strings.Contains(label, "cpu") {
			temps = append(temps, TemperatureInfo{Name: "CPU", Celsius: data.celsius})
			break // just the first CPU temp found
		}
	}
	return temps
}

func parseThermalZones(output string) []TemperatureInfo {
	type zoneData struct {
		name    string
		celsius float64
		hasTemp bool
	}
	zones := make(map[string]zoneData)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		path, value := parts[0], strings.TrimSpace(parts[1])
		zone := thermalZoneKey(path)
		if zone == "" {
			continue
		}
		data := zones[zone]
		if strings.HasSuffix(path, "/type") {
			data.name = value
		} else if strings.HasSuffix(path, "/temp") {
			raw, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			if raw > 1000 {
				raw = raw / 1000
			}
			data.celsius = raw
			data.hasTemp = true
		}
		zones[zone] = data
	}

	keys := make([]string, 0, len(zones))
	for key := range zones {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	temps := make([]TemperatureInfo, 0, len(keys))
	cpuZoneNames := map[string]bool{"x86_pkg_temp": true, "cpu-thermal": true, "coretemp": true, "k10temp": true, "cpu": true}
	cpuCount := 0
	for _, key := range keys {
		data := zones[key]
		if !data.hasTemp {
			continue
		}
		name := data.name
		if name == "" {
			name = key
		}
		if cpuZoneNames[strings.ToLower(name)] || cpuZoneNames[strings.ToLower(key)] {
			if cpuCount == 0 {
				name = "CPU"
			} else {
				name = fmt.Sprintf("CPU %d", cpuCount)
			}
			cpuCount++
		}
		temps = append(temps, TemperatureInfo{Name: name, Celsius: data.celsius})
	}
	return temps
}

func thermalZoneKey(path string) string {
	idx := strings.Index(path, "thermal_zone")
	if idx < 0 {
		return ""
	}
	rest := path[idx:]
	if slash := strings.Index(rest, "/"); slash >= 0 {
		return rest[:slash]
	}
	return rest
}

func getNetworkInfo(client *SSHClient) ([]NetworkInfo, error) {
	output, err := client.ExecuteCommand("cat /proc/net/dev")
	if err != nil {
		return nil, err
	}
	return parseNetworkDev(output), nil
}

func parseNetworkDev(output string) []NetworkInfo {
	var interfaces []NetworkInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		name := strings.TrimSpace(parts[0])
		if isVirtualNetworkInterface(name) {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}

		rxBytes, rxErr := strconv.ParseUint(fields[0], 10, 64)
		txBytes, txErr := strconv.ParseUint(fields[8], 10, 64)
		if rxErr != nil || txErr != nil {
			continue
		}

		interfaces = append(interfaces, NetworkInfo{Name: name, RXBytes: rxBytes, TXBytes: txBytes})
	}
	return interfaces
}

func isVirtualNetworkInterface(name string) bool {
	if name == "lo" {
		return true
	}
	virtualPrefixes := []string{"docker", "veth", "br-", "virbr", "tun", "tap", "tailscale", "zt", "wg"}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func computeNetworkRates(previous []NetworkInfo, current []NetworkInfo, elapsedSeconds float64) []NetworkInfo {
	if elapsedSeconds <= 0 {
		return current
	}
	prevByName := make(map[string]NetworkInfo, len(previous))
	for _, iface := range previous {
		prevByName[iface.Name] = iface
	}

	rated := make([]NetworkInfo, len(current))
	for i, iface := range current {
		rated[i] = iface
		prev, ok := prevByName[iface.Name]
		if !ok || iface.RXBytes < prev.RXBytes || iface.TXBytes < prev.TXBytes {
			continue
		}
		rated[i].RXBps = uint64(float64(iface.RXBytes-prev.RXBytes) / elapsedSeconds)
		rated[i].TXBps = uint64(float64(iface.TXBytes-prev.TXBytes) / elapsedSeconds)
	}
	return rated
}

func getProcessInfo(client *SSHClient) ([]ProcessInfo, error) {
	output, err := client.ExecuteCommand("ps -eo pid=,comm=,pcpu=,pmem= --sort=-pcpu | head -n 25")
	if err != nil {
		return nil, err
	}
	return parseTopProcesses(output, 25), nil
}

func parseTopProcesses(output string, limit int) []ProcessInfo {
	if limit <= 0 {
		return nil
	}
	processes := make([]ProcessInfo, 0, limit)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, pidErr := strconv.Atoi(fields[0])
		cpuPercent, cpuErr := strconv.ParseFloat(fields[len(fields)-2], 64)
		memPercent, memErr := strconv.ParseFloat(fields[len(fields)-1], 64)
		if pidErr != nil || cpuErr != nil || memErr != nil {
			continue
		}

		command := strings.Join(fields[1:len(fields)-2], " ")
		processes = append(processes, ProcessInfo{PID: pid, Command: command, CPUPercent: cpuPercent, MemPercent: memPercent})
		if len(processes) >= limit {
			break
		}
	}
	return processes
}
