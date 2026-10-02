package gpu

import (
	"encoding/csv"
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

// amdSmiInt decodes a numeric field from amd-smi's JSON, which reports "N/A"
// (a string) for sensors a card lacks. A plain int would fail the whole
// document on one such field and blank every GPU, so unreadable values read 0.
type amdSmiInt int

func (v *amdSmiInt) UnmarshalJSON(data []byte) error {
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		*v = 0
		return nil
	}
	*v = amdSmiInt(f)
	return nil
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
					Value amdSmiInt `json:"value"`
				} `json:"gfx_activity"`
			} `json:"usage"`
			Power struct {
				SocketPower struct {
					Value amdSmiInt `json:"value"`
				} `json:"socket_power"`
			} `json:"power"`
			Temperature struct {
				Hotspot struct {
					Value amdSmiInt `json:"value"`
				} `json:"hotspot"`
			} `json:"temperature"`
			MemUsage struct {
				TotalVRAM struct {
					Value amdSmiInt `json:"value"`
				} `json:"total_vram"`
				UsedVRAM struct {
					Value amdSmiInt `json:"value"`
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
			VRAMTotal:   int(metrics.MemUsage.TotalVRAM.Value),
			VRAMUsed:    int(metrics.MemUsage.UsedVRAM.Value),
			Utilization: int(metrics.Usage.GFXActivity.Value),
			PowerDraw:   int(metrics.Power.SocketPower.Value),
			Temperature: int(metrics.Temperature.Hotspot.Value),
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
	devices := parseRocmSmiCSV(output)
	if len(devices) == 0 {
		return nil, fmt.Errorf("no GPUs in rocm-smi output")
	}
	enrichAMDClocks(runCmd, devices)
	return devices, nil
}

// parseRocmSmiCSV reads rocm-smi's --csv report by column name. The column set
// and order vary between ROCm releases, and the tool prints human-readable
// warnings ("AMD GPU device(s) is/are in a low-power state...") to stdout ahead
// of the table, so neither a fixed position nor "first line is the header" holds.
func parseRocmSmiCSV(output string) []base.Device {
	lines := strings.Split(output, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "device,") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}

	records, err := csv.NewReader(strings.NewReader(strings.Join(lines[start:], "\n"))).ReadAll()
	if err != nil || len(records) < 2 {
		// ReadAll fails on ragged rows; re-read tolerantly.
		r := csv.NewReader(strings.NewReader(strings.Join(lines[start:], "\n")))
		r.FieldsPerRecord = -1
		if records, err = r.ReadAll(); err != nil || len(records) < 2 {
			return nil
		}
	}

	// col finds the first header containing every needle (case-insensitive).
	header := records[0]
	col := func(needles ...string) int {
		for i, h := range header {
			h = strings.ToLower(h)
			match := true
			for _, n := range needles {
				if !strings.Contains(h, n) {
					match = false
					break
				}
			}
			if match {
				return i
			}
		}
		return -1
	}
	cell := func(row []string, i int) (float64, bool) {
		if i < 0 || i >= len(row) {
			return 0, false
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(row[i]), 64)
		return v, err == nil
	}

	tempCol := col("temperature", "junction")
	if tempCol < 0 {
		tempCol = col("temperature")
	}
	powerCol := col("power (w)")
	useCol := col("gpu use")
	totalCol := col("vram total memory (b)")
	usedCol := col("vram total used memory (b)")
	nameCol := col("card series")

	var devices []base.Device
	for n, row := range records[1:] {
		if len(row) == 0 || !strings.HasPrefix(row[0], "card") {
			continue
		}
		device := base.Device{Index: n, Vendor: "amd", Name: "AMD GPU"}
		if idx, err := strconv.Atoi(strings.TrimPrefix(row[0], "card")); err == nil {
			device.Index = idx
		}
		if nameCol >= 0 && nameCol < len(row) && strings.TrimSpace(row[nameCol]) != "" {
			device.Name = strings.TrimSpace(row[nameCol])
		}
		if v, ok := cell(row, tempCol); ok {
			device.Temperature = int(v + 0.5)
		}
		if v, ok := cell(row, powerCol); ok {
			device.PowerDraw = int(v + 0.5)
		}
		if v, ok := cell(row, useCol); ok {
			device.Utilization = int(v + 0.5)
		}
		if v, ok := cell(row, totalCol); ok {
			device.VRAMTotal = int(v / 1048576) // bytes → MB
		}
		if v, ok := cell(row, usedCol); ok {
			device.VRAMUsed = int(v / 1048576)
		}
		devices = append(devices, device)
	}
	return devices
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
const DRMProbeCommand = "sh -c 'grep -H . " +
	"/sys/class/drm/card*/device/vendor " +
	"/sys/class/drm/card*/device/uevent " +
	"/sys/class/drm/card*/device/gpu_busy_percent " +
	"/sys/class/drm/card*/device/mem_info_vram_total " +
	"/sys/class/drm/card*/device/mem_info_vram_used " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/temp1_input " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/temp2_input " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/power1_average " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/power1_cap " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/freq1_input " +
	"/sys/class/drm/card*/device/hwmon/hwmon*/temp2_crit " +
	"/sys/class/drm/card*/device/pp_dpm_sclk " +
	"2>/dev/null || true'"

// drmCardPattern pulls the card number out of a /sys/class/drm path so readings
// from a card's hwmon subdirectory group with the card's own attributes.
var drmCardPattern = regexp.MustCompile(`/card(\d+)/`)

// drmCard is one card's sysfs attributes, keyed by file name (the final path
// element), plus its PCI slot for joining against lspci.
type drmCard struct {
	index int
	slot  string
	attrs map[string]string
	// sclkStates are the card's graphics-clock DPM states in MHz, lowest first;
	// sclkActive is the one the card is running now (0 if none is marked).
	sclkStates []int
	sclkActive int
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
		// No cap in sysfs leaves PowerLimit at 0, which the UI treats as unknown
		// and hides the power bar rather than draw it against an invented limit.
		card.applyClocks(&device)
		devices = append(devices, device)
	}
	return devices, nil
}

// discreteAMDCards returns the AMD cards sysfs reports, lowest card number
// first, excluding integrated graphics.
func discreteAMDCards(runCmd base.RunCmdFunc) []drmCard {
	return amdCards(runCmd, true)
}

// amdCards returns the AMD cards sysfs reports. With discreteOnly, integrated
// graphics (identified by their small VRAM carve-out) are excluded.
func amdCards(runCmd base.RunCmdFunc, discreteOnly bool) []drmCard {
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
		if attr == "pp_dpm_sclk" {
			// One line per state: "1: 821Mhz *", the star marking the active one.
			if mhz := parseDPMState(value); mhz > 0 {
				card.sclkStates = append(card.sclkStates, mhz)
				if strings.HasSuffix(value, "*") {
					card.sclkActive = mhz
				}
			}
			continue
		}
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
		if discreteOnly && card.intAttr("mem_info_vram_total")/1048576 < discreteVRAMFloorMB {
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

// parseDPMState reads the MHz out of a pp_dpm_sclk line such as "2: 2900Mhz *".
func parseDPMState(line string) int {
	_, rest, ok := strings.Cut(line, ":")
	if !ok {
		return 0
	}
	rest = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "*"))
	rest = strings.TrimSuffix(strings.ToLower(rest), "mhz")
	mhz, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0
	}
	return mhz
}

// applyClocks fills in the device's clock and throttle state from sysfs. amdgpu
// does not say why it slowed down, so the reason is inferred, and only when the
// evidence is concrete: a card well below its top clock under real load that is
// either pinned at its power cap or within a few degrees of its critical
// temperature. Anything vaguer is left unflagged rather than guessed at.
func (c drmCard) applyClocks(d *base.Device) {
	if len(c.sclkStates) > 0 {
		d.MaxClockMHz = c.sclkStates[len(c.sclkStates)-1]
		for _, mhz := range c.sclkStates {
			if mhz > d.MaxClockMHz {
				d.MaxClockMHz = mhz
			}
		}
	}
	d.ClockMHz = c.intAttr("freq1_input") / 1000000 // Hz → MHz
	if d.ClockMHz == 0 {
		d.ClockMHz = c.sclkActive
	}
	if d.MaxClockMHz == 0 || d.ClockMHz == 0 || d.Utilization < 50 || d.ClockMHz*100 > d.MaxClockMHz*65 {
		return
	}
	capW := c.intAttr("power1_cap") / 1000000
	if capW > 0 && d.PowerDraw*100 >= capW*95 {
		d.Throttle = append(d.Throttle, "power cap")
	}
	if crit := c.intAttr("temp2_crit") / 1000; crit > 0 && d.Temperature >= crit-5 {
		d.Throttle = append(d.Throttle, "thermal slowdown")
	}
}

// enrichAMDClocks adds clock and throttle data from sysfs to devices that came
// from rocm-smi, which reports neither. rocm-smi numbers its devices from zero
// in its own order, which does not match sysfs card numbers (an APU often holds
// card0 there and the Radeon card1 here), so devices are paired by position, and
// only when both sides list the same number of cards.
func enrichAMDClocks(runCmd base.RunCmdFunc, devices []base.Device) {
	cards := amdCards(runCmd, false)
	if len(cards) != len(devices) {
		return
	}
	for i := range devices {
		cards[i].applyClocks(&devices[i])
	}
}

// DRMProcessCommand dumps the DRM client accounting the kernel keeps in fdinfo,
// plus every process name, in one round trip (about 100 ms on a busy desktop).
// Run under sh so a login shell cannot trip on an unmatched glob. Only
// processes the SSH user may inspect appear, which in practice is their own.
const DRMProcessCommand = "sh -c '" +
	"grep -H -s -E \"^(drm-driver|drm-client-id|drm-pdev|drm-memory-vram|drm-total-vram|drm-engine-gfx|drm-engine-compute):\" /proc/[0-9]*/fdinfo/* 2>/dev/null; " +
	"grep -H -s . /proc/[0-9]*/comm 2>/dev/null; true'"

// Processes lists processes holding AMD GPU memory or running on its engines.
func (p AMDProvider) Processes(runCmd base.RunCmdFunc) []base.Process {
	out, err := runCmd(DRMProcessCommand)
	if err != nil {
		return nil
	}
	return parseDRMProcesses(out)
}

type drmClient struct {
	pid      int
	pdev     string
	driver   string
	vramKiB  uint64
	totalKiB uint64
	engineNs uint64
}

// parseDRMProcesses folds fdinfo lines into per-process totals. A process often
// has many file descriptors onto the same DRM client, each repeating identical
// counters, so clients are keyed by (pid, client-id) and counted once.
func parseDRMProcesses(out string) []base.Process {
	type key struct {
		pid    int
		client string
	}
	clients := map[key]*drmClient{}
	names := map[int]string{}
	var order []key

	// fdinfo is grouped by file, so the client id arrives mid-group; remember each
	// fd's lines and resolve them once the id is known.
	type fdState struct {
		pid   int
		id    string
		attrs map[string]string
	}
	fds := map[string]*fdState{}
	var fdOrder []string

	for _, line := range strings.Split(out, "\n") {
		path, rest, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(path, "/proc/") {
			continue
		}
		parts := strings.Split(path, "/") // "", "proc", PID, "fdinfo", FD | "", "proc", PID, "comm"
		if len(parts) < 4 {
			continue
		}
		pid, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		if parts[3] == "comm" {
			names[pid] = strings.TrimSpace(rest)
			continue
		}
		if parts[3] != "fdinfo" {
			continue
		}
		k, v, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		fd := fds[path]
		if fd == nil {
			fd = &fdState{pid: pid, attrs: map[string]string{}}
			fds[path] = fd
			fdOrder = append(fdOrder, path)
		}
		fd.attrs[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	for _, path := range fdOrder {
		fd := fds[path]
		if fd.attrs["drm-driver"] != "amdgpu" {
			continue
		}
		k := key{fd.pid, fd.attrs["drm-client-id"]}
		if clients[k] != nil {
			continue
		}
		c := &drmClient{pid: fd.pid, pdev: fd.attrs["drm-pdev"]}
		c.vramKiB = parseDRMSizeKiB(fd.attrs["drm-memory-vram"])
		c.totalKiB = parseDRMSizeKiB(fd.attrs["drm-total-vram"])
		for _, engine := range []string{"drm-engine-gfx", "drm-engine-compute"} {
			ns, _ := strconv.ParseUint(strings.TrimSuffix(fd.attrs[engine], " ns"), 10, 64)
			c.engineNs += ns
		}
		clients[k] = c
		order = append(order, k)
	}

	// Sum a process's clients per card.
	type pk struct {
		pid  int
		pdev string
	}
	merged := map[pk]*base.Process{}
	var mergedOrder []pk
	for _, k := range order {
		c := clients[k]
		mem := c.vramKiB
		if mem == 0 {
			mem = c.totalKiB // older kernels only report the allocated figure
		}
		id := pk{c.pid, c.pdev}
		proc := merged[id]
		if proc == nil {
			name := names[c.pid]
			if name == "" {
				name = "pid " + strconv.Itoa(c.pid)
			}
			proc = &base.Process{GPU: shortPCI(c.pdev), PID: c.pid, Name: name, UtilPct: -1}
			merged[id] = proc
			mergedOrder = append(mergedOrder, id)
		}
		proc.VRAMMB += int(mem / 1024)
		proc.EngineNs += c.engineNs
	}

	procs := make([]base.Process, 0, len(mergedOrder))
	for _, id := range mergedOrder {
		// A client that holds no memory and has never run work is just an open
		// device node (compositors and browsers keep dozens); listing them buries
		// the processes that matter.
		if p := merged[id]; p.VRAMMB > 0 || p.EngineNs > 0 {
			procs = append(procs, *p)
		}
	}
	return procs
}

// parseDRMSizeKiB reads fdinfo sizes like "12 KiB" or "2 MiB"; a bare number is
// bytes.
func parseDRMSizeKiB(v string) uint64 {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return 0
	}
	n, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0
	}
	if len(fields) == 1 {
		return n / 1024
	}
	switch fields[1] {
	case "KiB":
		return n
	case "MiB":
		return n * 1024
	case "GiB":
		return n * 1024 * 1024
	}
	return n / 1024
}

// shortPCI trims the PCI domain ("0000:c4:00.0" → "c4:00.0").
func shortPCI(pdev string) string {
	if strings.Count(pdev, ":") == 2 {
		return pdev[strings.Index(pdev, ":")+1:]
	}
	return pdev
}
