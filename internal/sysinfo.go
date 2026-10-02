package internal

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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

	// Extended sensors (rendered only in single-host view).
	Swap     SwapInfo
	GPUProcs []GPUProcessInfo
	Load     LoadInfo
	DiskIO   []DiskIOInfo
	Fans     []FanInfo

	// Uptime is how long the host has been running (0 if unavailable).
	Uptime time.Duration
}

type CPUInfo struct {
	Model        string
	Count        string
	Usage        string
	UsagePercent float64
	Cores        []CPUCoreInfo
	// FreqMHz is the mean current clock across cores and MaxFreqMHz the highest
	// rated clock; 0 when cpufreq is not exposed.
	FreqMHz    int
	MaxFreqMHz int
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
	ClockMHz    int
	MaxClockMHz int
	Throttle    []string
}

// GPUProcessInfo is one process using a GPU.
type GPUProcessInfo struct {
	GPU      string
	PID      int
	Name     string
	VRAMMB   int
	EngineNs uint64  // cumulative busy time, when the driver reports it
	UtilPct  float64 // derived by the UI layer (or reported directly); -1 = unknown
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

	// Exact byte counts, for trend forecasting; the string fields above are
	// rounded for display.
	TotalBytes uint64
	UsedBytes  uint64
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

type SwapInfo struct {
	Total        int // in MB
	Used         int // in MB
	UsagePercent float64
}

type LoadInfo struct {
	Load1   float64
	Load5   float64
	Load15  float64
	Running int
	Total   int
}

type DiskIOInfo struct {
	Device     string
	ReadBytes  uint64 // cumulative counter
	WriteBytes uint64 // cumulative counter
	ReadBps    uint64 // derived rate (filled by UI layer)
	WriteBps   uint64 // derived rate (filled by UI layer)
}

type FanInfo struct {
	Name string
	RPM  int
}

func GatherSystemInfo(client *SSHClient) (*SystemInfo, error) {
	info := &SystemInfo{}

	top := getTopSnapshot(client)

	cpuInfo, err := getCPUInfo(client, top)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU info: %w", err)
	}
	cpuInfo.FreqMHz, cpuInfo.MaxFreqMHz = getCPUFreq(client)
	info.CPU = cpuInfo

	gpuInfo, _ := getGPUInfo(client)
	info.GPUs = gpuInfo
	if len(gpuInfo) > 0 {
		info.GPUProcs = getGPUProcesses(client)
	}

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

	if processInfo, err := getProcessInfo(client, top); err == nil {
		info.Processes = processInfo
	}

	// Extended sensors (best-effort; rendered only in single-host view).
	if swap, err := getSwapInfo(client); err == nil {
		info.Swap = swap
	}
	if load, err := getLoadInfo(client); err == nil {
		info.Load = load
	}
	if diskIO, err := getDiskIOInfo(client); err == nil {
		info.DiskIO = diskIO
	}
	if fans, err := getFanInfo(client); err == nil {
		info.Fans = fans
	}
	if uptime, err := getUptimeInfo(client); err == nil {
		info.Uptime = uptime
	}

	return info, nil
}

func getUptimeInfo(client *SSHClient) (time.Duration, error) {
	output, err := client.ExecuteCommand("cat /proc/uptime")
	if err != nil {
		return 0, err
	}
	return parseUptime(output), nil
}

// parseUptime reads /proc/uptime, whose first field is the uptime in seconds
// (e.g. "350735.47 234388.90").
func parseUptime(output string) time.Duration {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func getCPUInfo(client *SSHClient, top string) (CPUInfo, error) {
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

	output = top
	if output == "" {
		// top's first sample is an average since boot, not current load, so take
		// two and let parseTopCPUUsage keep only the last (a real one-second
		// interval).
		output, _ = client.ExecuteCommand("env LC_ALL=C top -bn2 -d 1 -1 | grep -E '^(%Cpu|CPU:)'")
	}
	if output != "" {
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

// parseTopCPUUsage extracts aggregate and per-core usage from top's CPU lines.
// When top ran several iterations the output holds one block per iteration; only
// the last block is kept, since earlier ones are since-boot averages.
func parseTopCPUUsage(output string) (float64, []CPUCoreInfo) {
	var aggregate float64
	var cores []CPUCoreInfo
	sawAggregate := false
	seenCores := map[int]bool{}

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
			if sawAggregate {
				cores, seenCores = nil, map[int]bool{} // a new iteration begins
			}
			sawAggregate = true
			aggregate = usage
			continue
		}

		if strings.HasPrefix(line, "%Cpu") {
			label := strings.TrimPrefix(strings.Fields(line)[0], "%Cpu")
			label = strings.TrimSuffix(label, ":")
			if idx, err := strconv.Atoi(label); err == nil {
				if seenCores[idx] { // per-core mode has no aggregate line; a repeat marks a new iteration
					cores, seenCores, aggregate = nil, map[int]bool{}, 0
				}
				seenCores[idx] = true
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

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
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
		return clampPercent(100 - idle), true
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
			ClockMHz:    dev.ClockMHz,
			MaxClockMHz: dev.MaxClockMHz,
			Throttle:    dev.Throttle,
		}
	}

	return gpus, nil
}

func getGPUProcesses(client *SSHClient) []GPUProcessInfo {
	procs := gpu.QueryProcesses(func(cmd string) (string, error) { return client.ExecuteCommand(cmd) })
	out := make([]GPUProcessInfo, len(procs))
	for i, p := range procs {
		out[i] = GPUProcessInfo{GPU: p.GPU, PID: p.PID, Name: p.Name, VRAMMB: p.VRAMMB, EngineNs: p.EngineNs, UtilPct: float64(p.UtilPct)}
	}
	return out
}

// cpuFreqCommand reads every core's current clock and rated maximum in one call.
// It runs under sh so a login shell can't abort on the cpu* glob.
const cpuFreqCommand = "sh -c 'grep -H . /sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq /sys/devices/system/cpu/cpu[0-9]*/cpufreq/cpuinfo_max_freq 2>/dev/null; true'"

func getCPUFreq(client *SSHClient) (cur, max int) {
	output, err := client.ExecuteCommand(cpuFreqCommand)
	if err != nil {
		return 0, 0
	}
	return parseCPUFreq(output)
}

// parseCPUFreq averages scaling_cur_freq over the cores and takes the highest
// cpuinfo_max_freq. The kernel reports kHz.
func parseCPUFreq(output string) (curMHz, maxMHz int) {
	var sum, n, maxKHz int
	for _, line := range strings.Split(output, "\n") {
		path, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		khz, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || khz <= 0 {
			continue
		}
		switch {
		case strings.HasSuffix(path, "scaling_cur_freq"):
			sum += khz
			n++
		case strings.HasSuffix(path, "cpuinfo_max_freq"):
			if khz > maxKHz {
				maxKHz = khz
			}
		}
	}
	if n > 0 {
		curMHz = sum / n / 1000
	}
	return curMHz, maxKHz / 1000
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
	output, err := client.ExecuteCommand("df -kP | grep -E '^/dev/'")
	if err != nil {
		return nil, err
	}

	return parseDF(output), nil
}

// parseDF parses `df -kP` rows for real block devices. The same device often
// appears several times (btrfs subvolumes, bind mounts) with identical usage, so
// each device is reported once under its shortest mount point. Loop devices —
// snap/appimage squashfs mounts that are always 100% full — are noise.
func parseDF(output string) []DiskInfo {
	var disks []DiskInfo
	byDevice := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 6 || strings.HasPrefix(parts[0], "/dev/loop") {
			continue
		}
		totalKiB, err1 := strconv.ParseUint(parts[1], 10, 64)
		usedKiB, err2 := strconv.ParseUint(parts[2], 10, 64)
		availKiB, err3 := strconv.ParseUint(parts[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		disk := DiskInfo{
			Device:       parts[0],
			Size:         humanKiB(totalKiB),
			Used:         humanKiB(usedKiB),
			Available:    humanKiB(availKiB),
			UsagePercent: parts[4],
			MountPoint:   strings.Join(parts[5:], " "), // mount points may contain spaces
			TotalBytes:   totalKiB * 1024,
			UsedBytes:    usedKiB * 1024,
		}
		// Docker injects /etc/hostname, /etc/hosts and /etc/resolv.conf as bind
		// mounts of host files: df reports them as the backing block device
		// (e.g. /dev/nvme0n1p4) with a bogus mount point, and the per-device
		// dedup below would let that row stand in for the real filesystem.
		if containerInjectedMount(disk.MountPoint) {
			continue
		}
		if i, seen := byDevice[disk.Device]; seen {
			if len(disk.MountPoint) < len(disks[i].MountPoint) {
				disks[i] = disk
			}
			continue
		}
		byDevice[disk.Device] = len(disks)
		disks = append(disks, disk)
	}
	return disks
}

// containerInjectedMount reports whether a mount point is one of the files
// Docker always injects into a container. They are bind mounts of host files,
// so df attributes them to the host's root block device with a mount point that
// does not describe a real filesystem.
func containerInjectedMount(mountPoint string) bool {
	switch mountPoint {
	case "/etc/hostname", "/etc/hosts", "/etc/resolv.conf", "/etc/hostid":
		return true
	}
	return false
}

// humanKiB formats a size like `df -h`: binary units, one decimal below 10 and
// whole numbers above, rounding up as coreutils does.
func humanKiB(kib uint64) string {
	units := []string{"K", "M", "G", "T", "P"}
	v := float64(kib)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 && i > 0 {
		return fmt.Sprintf("%.1f%s", math.Ceil(v*10)/10, units[i])
	}
	return fmt.Sprintf("%.0f%s", math.Ceil(v), units[i])
}

func getTemperatureInfo(client *SSHClient) ([]TemperatureInfo, error) {
	// grep exits non-zero if any single zone is unreadable, yet still prints the
	// rest, so judge by what parses rather than by the exit status.
	output, _ := client.ExecuteCommand("grep -H . /sys/class/thermal/thermal_zone*/type /sys/class/thermal/thermal_zone*/temp")
	temps := parseThermalZones(output)
	hasCPU := false
	for _, t := range temps {
		if strings.HasPrefix(t.Name, "CPU") {
			hasCPU = true
			break
		}
	}
	if !hasCPU {
		// Boards that only expose generic ACPI zones (acpitz) still publish the
		// real CPU die temperature through hwmon (k10temp/coretemp).
		if hw, err := getHwmonCPUTemp(client); err == nil {
			temps = append(hw, dropGenericACPIZones(temps)...)
		}
	}
	if len(temps) == 0 {
		return nil, fmt.Errorf("no temperature sensors")
	}
	return temps, nil
}

// dropGenericACPIZones removes firmware "acpitz" zones. They are unlabelled
// board-level readings that usually mirror the CPU or report a fixed
// placeholder, so once a real CPU sensor is known they only add duplicate rows
// and duplicate alerts.
func dropGenericACPIZones(temps []TemperatureInfo) []TemperatureInfo {
	kept := temps[:0:0]
	for _, t := range temps {
		if !strings.HasPrefix(t.Name, "acpitz") {
			kept = append(kept, t)
		}
	}
	return kept
}

func getHwmonCPUTemp(client *SSHClient) ([]TemperatureInfo, error) {
	// find (not a shell glob) keeps this shell-agnostic — see getFanInfo.
	output, err := client.ExecuteCommand("find -L /sys/class/hwmon -maxdepth 2 \\( -name 'temp*_input' -o -name 'temp*_label' \\) -exec grep -H . {} + 2>/dev/null || true")
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

	// Iterate sensor keys in order so the chosen CPU temp is deterministic
	// (e.g. temp1 "Package id 0" before temp2 "Core 0"), not map-order dependent.
	keys := make([]string, 0, len(sensors))
	for key := range sensors {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var temps []TemperatureInfo
	// Pick the most credible CPU die sensor rather than the first label that
	// happens to mention "cpu". Super-I/O chips (nct6799, it87, …) publish
	// board channels whose labels look CPU-ish — "CPUTIN", "PCH_CPU_TEMP",
	// "PCH_CHIP_CPU_MAX_TEMP" — and unwired channels read 0, which is how a
	// healthy k10temp (Tctl) gets masked by a 0 °C reading.
	bestRank, best := 2, ""
	for _, key := range keys {
		data := sensors[key]
		if !data.hasTemp || data.celsius <= 0 {
			continue // a CPU die never reports 0 °C
		}
		rank := hwmonCPURank(data.label)
		if rank < 0 || rank >= bestRank {
			continue // ties keep the earlier key: order stays deterministic
		}
		bestRank, best = rank, key
	}
	if best == "" {
		return nil
	}
	temps = append(temps, TemperatureInfo{Name: "CPU", Celsius: sensors[best].celsius})
	return temps
}

// hwmonCPURank scores a hwmon label by how likely it is the real CPU die
// temperature: 0 = CPU package/die sensor (k10temp, coretemp), 1 = other
// CPU-ish label, -1 = not a CPU temperature at all.
func hwmonCPURank(label string) int {
	label = strings.ToLower(strings.TrimSpace(label))
	switch {
	case label == "":
		return -1
	case strings.Contains(label, "pch"), strings.HasSuffix(label, "tin"):
		// PCH_CPU_TEMP / PCH_CHIP_CPU_MAX_TEMP / CPUTIN / SYSTIN / AUXTIN are
		// super-I/O board channels, not the CPU die.
		return -1
	case label == "tctl", label == "tdie", label == "package id 0", label == "physical id 0":
		return 0
	case strings.HasPrefix(label, "tccd"), strings.HasPrefix(label, "core "):
		return 0
	case label == "cpu", strings.Contains(label, "package"), strings.Contains(label, "cpu"):
		return 1
	}
	return -1
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
		if isControlThermalZone(name) {
			continue
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

	// Several zones commonly share a type (three "acpitz"); number the repeats so
	// the rows are distinguishable.
	total := map[string]int{}
	for _, t := range temps {
		total[t.Name]++
	}
	seen := map[string]int{}
	for i, t := range temps {
		if total[t.Name] > 1 {
			seen[t.Name]++
			if seen[t.Name] > 1 {
				temps[i].Name = fmt.Sprintf("%s %d", t.Name, seen[t.Name])
			}
		}
	}
	return temps
}

// isControlThermalZone reports whether a thermal zone is an ACPI DPTF control
// node rather than a real temperature sensor. INT3400 is the Intel Dynamic
// Platform & Thermal Framework manager: it exposes a thermal zone but reports a
// fixed sentinel value (commonly 20°C), so it's noise in the sensor list.
func isControlThermalZone(name string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(name)), "INT3400")
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
	virtualPrefixes := []string{"docker", "veth", "br-", "virbr", "vnet", "tun", "tap", "tailscale", "zt", "wg", "lxc", "cni", "flannel", "cali", "podman"}
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

// topSnapshotCommand takes two top samples one second apart and keeps only the
// second: the first covers the time since boot (or since each process started),
// the second covers the last second, which is what a live monitor should show.
// LC_ALL=C pins the decimal separator, since a comma locale would otherwise turn
// "12,5" into 125. Output is the summary and the head of the process table.
const topSnapshotCommand = "env LC_ALL=C top -bn2 -d 1 -1 -w 512 -o %CPU | awk '/^top -/{n++} n==2' | head -n 120"

// getTopSnapshot returns the live top output, or "" when top lacks the needed
// options (BusyBox, older procps) and callers should use their fallbacks.
func getTopSnapshot(client *SSHClient) string {
	output, err := client.ExecuteCommand(topSnapshotCommand)
	if err != nil || len(parseTopTable(output, 1)) == 0 {
		return ""
	}
	return output
}

// parseTopTable reads the process table from top's batch output by column name,
// so it doesn't depend on exactly which columns this top version prints.
func parseTopTable(output string, limit int) []ProcessInfo {
	var processes []ProcessInfo
	pidCol, cpuCol, memCol, cmdCol := -1, -1, -1, -1
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		if pidCol < 0 {
			if fields[0] != "PID" {
				continue
			}
			for i, f := range fields {
				switch f {
				case "PID":
					pidCol = i
				case "%CPU":
					cpuCol = i
				case "%MEM":
					memCol = i
				case "COMMAND":
					cmdCol = i
				}
			}
			if cpuCol < 0 || memCol < 0 || cmdCol < 0 {
				return nil
			}
			continue
		}
		if len(fields) <= cmdCol {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[pidCol])
		cpu, cpuErr := strconv.ParseFloat(fields[cpuCol], 64)
		mem, memErr := strconv.ParseFloat(fields[memCol], 64)
		if pidErr != nil || cpuErr != nil || memErr != nil {
			continue
		}
		processes = append(processes, ProcessInfo{PID: pid, Command: strings.Join(fields[cmdCol:], " "), CPUPercent: cpu, MemPercent: mem})
		if len(processes) >= limit {
			break
		}
	}
	return processes
}

func getProcessInfo(client *SSHClient, top string) ([]ProcessInfo, error) {
	if processes := parseTopTable(top, 25); len(processes) > 0 {
		return processes, nil
	}
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

func getSwapInfo(client *SSHClient) (SwapInfo, error) {
	info := SwapInfo{}
	output, err := client.ExecuteCommand("free -m | grep -i Swap:")
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

func getLoadInfo(client *SSHClient) (LoadInfo, error) {
	output, err := client.ExecuteCommand("cat /proc/loadavg")
	if err != nil {
		return LoadInfo{}, err
	}
	return parseLoadAvg(output), nil
}

// parseLoadAvg parses a /proc/loadavg line: "0.52 0.58 0.59 1/523 12345".
func parseLoadAvg(output string) LoadInfo {
	info := LoadInfo{}
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) >= 3 {
		info.Load1, _ = strconv.ParseFloat(fields[0], 64)
		info.Load5, _ = strconv.ParseFloat(fields[1], 64)
		info.Load15, _ = strconv.ParseFloat(fields[2], 64)
	}
	if len(fields) >= 4 {
		if rt := strings.SplitN(fields[3], "/", 2); len(rt) == 2 {
			info.Running, _ = strconv.Atoi(rt[0])
			info.Total, _ = strconv.Atoi(rt[1])
		}
	}
	return info
}

func getDiskIOInfo(client *SSHClient) ([]DiskIOInfo, error) {
	output, err := client.ExecuteCommand("cat /proc/diskstats")
	if err != nil {
		return nil, err
	}
	return parseDiskStats(output), nil
}

// parseDiskStats parses /proc/diskstats. Per the kernel docs the relevant
// columns are: 3=device name, 6=sectors read, 10=sectors written. Sectors are
// 512 bytes. Partitions and zero-traffic loop/ram devices are skipped.
func parseDiskStats(output string) []DiskIOInfo {
	const sectorSize = 512
	var stats []DiskIOInfo
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		device := fields[2]
		if strings.HasPrefix(device, "loop") || strings.HasPrefix(device, "ram") || isDerivedBlockDevice(device) {
			continue
		}
		readSectors, err1 := strconv.ParseUint(fields[5], 10, 64)
		writeSectors, err2 := strconv.ParseUint(fields[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		stats = append(stats, DiskIOInfo{
			Device:     device,
			ReadBytes:  readSectors * sectorSize,
			WriteBytes: writeSectors * sectorSize,
		})
	}
	return stats
}

// partitionPattern matches partitions of SCSI/virtio/IDE disks (sda1), and of
// NVMe/MMC disks (nvme0n1p2, mmcblk0p1).
var partitionPattern = regexp.MustCompile(`^((s|v|xv|h)d[a-z]+[0-9]+|(nvme[0-9]+n[0-9]+|mmcblk[0-9]+)p[0-9]+)$`)

// isDerivedBlockDevice reports whether a device's traffic is already counted on
// another entry: partitions repeat their parent disk, and device-mapper/md
// volumes repeat the disks beneath them. Summing them would inflate totals.
func isDerivedBlockDevice(name string) bool {
	return partitionPattern.MatchString(name) || strings.HasPrefix(name, "dm-") || strings.HasPrefix(name, "md")
}

func getFanInfo(client *SSHClient) ([]FanInfo, error) {
	// Use find (not a shell glob) so the command is shell-agnostic: a zsh login
	// shell aborts on an unmatched glob, and drivers like nct6775 expose
	// fan*_input with no fan*_label, which would kill a `grep .../fan*_label`.
	output, err := client.ExecuteCommand("find -L /sys/class/hwmon -maxdepth 2 \\( -name 'fan*_input' -o -name 'fan*_label' \\) -exec grep -H . {} + 2>/dev/null || true")
	if err != nil {
		return nil, err
	}
	return parseFans(output), nil
}

// parseFans parses paired hwmon fan*_label / fan*_input files (same shape as
// parseHwmonTemps). Fans reporting 0 RPM are omitted.
func parseFans(output string) []FanInfo {
	type fanData struct {
		label string
		rpm   int
		hasV  bool
	}
	fans := make(map[string]fanData)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		path, value := parts[0], strings.TrimSpace(parts[1])

		relPath := strings.TrimPrefix(path, "/sys/class/hwmon/")
		lastSlash := strings.LastIndex(relPath, "/")
		if lastSlash < 0 {
			continue
		}
		dir := relPath[:lastSlash]
		file := relPath[lastSlash+1:]
		if !strings.HasPrefix(file, "fan") {
			continue
		}
		sensorPart := file
		if idx := strings.IndexAny(sensorPart, "_."); idx >= 0 {
			sensorPart = sensorPart[:idx]
		}
		key := dir + "/" + sensorPart

		data := fans[key]
		if strings.HasSuffix(path, "_label") {
			data.label = value
		} else if strings.HasSuffix(path, "_input") {
			rpm, err := strconv.Atoi(value)
			if err != nil {
				continue
			}
			data.rpm = rpm
			data.hasV = true
		}
		fans[key] = data
	}

	keys := make([]string, 0, len(fans))
	for key := range fans {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]FanInfo, 0, len(keys))
	for _, key := range keys {
		data := fans[key]
		if !data.hasV || data.rpm <= 0 {
			continue
		}
		name := data.label
		if name == "" {
			// Fall back to the fanN identifier.
			name = key[strings.LastIndex(key, "/")+1:]
		}
		result = append(result, FanInfo{Name: name, RPM: data.rpm})
	}
	return result
}
