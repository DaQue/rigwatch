package internal

import (
	"strings"
	"testing"
)

func TestParseTopCPUUsageParsesAggregateAndCores(t *testing.T) {
	output := `%Cpu(s):  12.5 us,  2.5 sy,  0.0 ni,  85.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
%Cpu0  :  10.0 us,  5.0 sy,  0.0 ni,  85.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
%Cpu1  :  30.0 us, 10.0 sy,  0.0 ni,  60.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
%Cpu2  :   0.0 us,  1.0 sy,  0.0 ni,  99.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st`

	aggregate, cores := parseTopCPUUsage(output)
	if aggregate != 15.0 {
		t.Fatalf("aggregate = %.1f, want 15.0", aggregate)
	}
	if len(cores) != 3 {
		t.Fatalf("cores len = %d, want 3: %#v", len(cores), cores)
	}
	if cores[0].Index != 0 || cores[0].UsagePercent != 15.0 {
		t.Fatalf("core 0 = %#v, want index 0 usage 15.0", cores[0])
	}
	if cores[1].Index != 1 || cores[1].UsagePercent != 40.0 {
		t.Fatalf("core 1 = %#v, want index 1 usage 40.0", cores[1])
	}
	if cores[2].Index != 2 || cores[2].UsagePercent != 1.0 {
		t.Fatalf("core 2 = %#v, want index 2 usage 1.0", cores[2])
	}
}

func TestParseTopCPUUsageHandlesBusyboxStyleCpuLine(t *testing.T) {
	output := `CPU: 25.0% usr 5.0% sys 0.0% nic 70.0% idle 0.0% io 0.0% irq 0.0% sirq`

	aggregate, cores := parseTopCPUUsage(output)
	if aggregate != 30.0 {
		t.Fatalf("aggregate = %.1f, want 30.0", aggregate)
	}
	if len(cores) != 0 {
		t.Fatalf("cores = %#v, want none", cores)
	}
}

func TestParseTopCPUUsageAveragesCoresWhenAggregateMissing(t *testing.T) {
	output := `%Cpu0  : 10.0 us,  0.0 sy,  0.0 ni, 90.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
%Cpu1  : 40.0 us,  0.0 sy,  0.0 ni, 60.0 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st`

	aggregate, cores := parseTopCPUUsage(output)
	if aggregate != 25.0 {
		t.Fatalf("aggregate = %.1f, want average 25.0", aggregate)
	}
	if len(cores) != 2 {
		t.Fatalf("cores len = %d, want 2", len(cores))
	}
}

func TestNormalizeCPUCoresFillsEveryCoreSlot(t *testing.T) {
	cores := normalizeCPUCores("4", []CPUCoreInfo{
		{Index: 2, UsagePercent: 22},
		{Index: 0, UsagePercent: 10},
	})
	if len(cores) != 4 {
		t.Fatalf("cores len = %d, want 4: %#v", len(cores), cores)
	}
	for i, core := range cores {
		if core.Index != i {
			t.Fatalf("core[%d].Index = %d, want %d: %#v", i, core.Index, i, cores)
		}
	}
	if cores[0].UsagePercent != 10 || cores[1].UsagePercent != 0 || cores[2].UsagePercent != 22 || cores[3].UsagePercent != 0 {
		t.Fatalf("usage values = %#v, want missing cores allocated at 0%%", cores)
	}
}

func TestNormalizeCPUCoresKeepsReportedCoresWhenCountUnavailable(t *testing.T) {
	cores := normalizeCPUCores("", []CPUCoreInfo{{Index: 3, UsagePercent: 33}})
	if len(cores) != 1 || cores[0].Index != 3 || cores[0].UsagePercent != 33 {
		t.Fatalf("cores = %#v, want reported cores unchanged", cores)
	}
}

func TestParseThermalZonesPairsTypeAndTemp(t *testing.T) {
	output := `/sys/class/thermal/thermal_zone0/type:x86_pkg_temp
/sys/class/thermal/thermal_zone0/temp:55000
/sys/class/thermal/thermal_zone1/type:nvme
/sys/class/thermal/thermal_zone1/temp:42123`

	got := parseThermalZones(output)
	if len(got) != 2 {
		t.Fatalf("thermal zones = %#v, want 2", got)
	}
	if got[0].Name != "CPU" || got[0].Celsius != 55.0 {
		t.Fatalf("first thermal zone = %#v, want CPU 55.0C", got[0])
	}
	if got[1].Name != "nvme" || got[1].Celsius != 42.123 {
		t.Fatalf("second thermal zone = %#v, want nvme 42.123C", got[1])
	}
}

func TestParseThermalZonesFiltersDPTFManager(t *testing.T) {
	output := `/sys/class/thermal/thermal_zone0/type:INT3400 Thermal
/sys/class/thermal/thermal_zone0/temp:20000
/sys/class/thermal/thermal_zone1/type:nvme
/sys/class/thermal/thermal_zone1/temp:42123`

	got := parseThermalZones(output)
	if len(got) != 1 {
		t.Fatalf("thermal zones = %#v, want 1 (INT3400 filtered)", got)
	}
	if got[0].Name != "nvme" {
		t.Fatalf("remaining zone = %#v, want nvme", got[0])
	}
}

func TestParseNetworkDevFiltersVirtualInterfaces(t *testing.T) {
	output := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
  eth0: 123456 100 0 0 0 0 0 0 654321 120 0 0 0 0 0 0
 docker0: 500 1 0 0 0 0 0 0 600 2 0 0 0 0 0 0
 vethabc: 700 1 0 0 0 0 0 0 800 2 0 0 0 0 0 0`

	got := parseNetworkDev(output)
	if len(got) != 1 {
		t.Fatalf("network interfaces = %#v, want one physical interface", got)
	}
	if got[0].Name != "eth0" || got[0].RXBytes != 123456 || got[0].TXBytes != 654321 {
		t.Fatalf("network interface = %#v, want eth0 counters", got[0])
	}
}

func TestComputeNetworkRatesUsesCounterDelta(t *testing.T) {
	prev := []NetworkInfo{{Name: "eth0", RXBytes: 1000, TXBytes: 2000}}
	curr := []NetworkInfo{{Name: "eth0", RXBytes: 7000, TXBytes: 5000}}

	got := computeNetworkRates(prev, curr, 3)
	if len(got) != 1 {
		t.Fatalf("network rates = %#v, want one interface", got)
	}
	if got[0].RXBps != 2000 || got[0].TXBps != 1000 {
		t.Fatalf("network rates = %#v, want 2000 RX B/s and 1000 TX B/s", got[0])
	}
}

func TestParseTopProcessesSortsAndTruncatesRows(t *testing.T) {
	output := `  PID COMMAND         %CPU %MEM
 1001 postgres        42.5 12.3
 1002 verylongcommand 10.0  2.5
 1003 sshd             0.1  0.2`

	got := parseTopProcesses(output, 2)
	if len(got) != 2 {
		t.Fatalf("processes len = %d, want 2: %#v", len(got), got)
	}
	if got[0].PID != 1001 || got[0].Command != "postgres" || got[0].CPUPercent != 42.5 || got[0].MemPercent != 12.3 {
		t.Fatalf("first process = %#v, want postgres row", got[0])
	}
	if got[1].PID != 1002 || got[1].Command != "verylongcommand" {
		t.Fatalf("second process = %#v, want verylongcommand row", got[1])
	}
}

func TestParseHwmonTempsFindsCPUPackageTemp(t *testing.T) {
	output := `/sys/class/hwmon/hwmon0/name:coretemp
/sys/class/hwmon/hwmon0/temp1_label:Package id 0
/sys/class/hwmon/hwmon0/temp1_input:55000
/sys/class/hwmon/hwmon0/temp2_label:Core 0
/sys/class/hwmon/hwmon0/temp2_input:42000
/sys/class/hwmon/hwmon0/temp3_label:Core 1
/sys/class/hwmon/hwmon0/temp3_input:43000`

	got := parseHwmonTemps(output)
	if len(got) != 1 {
		t.Fatalf("hwmon temps = %#v, want 1 (CPU package)", got)
	}
	if got[0].Name != "CPU" || got[0].Celsius != 55.0 {
		t.Fatalf("first hwmon temp = %#v, want CPU 55.0C", got[0])
	}
}

func TestParseHwmonTempsReturnsEmptyWhenNoCPUSensor(t *testing.T) {
	output := `/sys/class/hwmon/hwmon0/name:nvme
/sys/class/hwmon/hwmon0/temp1_label:Composite
/sys/class/hwmon/hwmon0/temp1_input:35000`

	got := parseHwmonTemps(output)
	if len(got) != 0 {
		t.Fatalf("hwmon temps = %#v, want empty (no CPU sensor)", got)
	}
}

func TestParseLoadAvgParsesAllFields(t *testing.T) {
	got := parseLoadAvg("0.52 0.58 0.59 2/523 12345\n")
	if got.Load1 != 0.52 || got.Load5 != 0.58 || got.Load15 != 0.59 {
		t.Fatalf("load averages = %+v", got)
	}
	if got.Running != 2 || got.Total != 523 {
		t.Fatalf("running/total = %d/%d, want 2/523", got.Running, got.Total)
	}
}

func TestParseDiskStatsConvertsSectorsToBytesAndSkipsLoop(t *testing.T) {
	// fields (0-indexed): 2=name, 5=sectors read, 9=sectors written
	output := `   8       0 sda 100 0 200 50 80 0 400 30 0 0
   7       0 loop0 1 0 2 0 3 0 4 0 0 0`
	got := parseDiskStats(output)
	if len(got) != 1 {
		t.Fatalf("expected loop device skipped, got %d devices: %+v", len(got), got)
	}
	if got[0].Device != "sda" {
		t.Fatalf("device = %q", got[0].Device)
	}
	if got[0].ReadBytes != 200*512 || got[0].WriteBytes != 400*512 {
		t.Fatalf("bytes = r%d w%d, want r%d w%d", got[0].ReadBytes, got[0].WriteBytes, 200*512, 400*512)
	}
}

func TestParseFansPairsLabelAndInputAndDropsZero(t *testing.T) {
	output := `/sys/class/hwmon/hwmon2/fan1_label:CPU Fan
/sys/class/hwmon/hwmon2/fan1_input:1200
/sys/class/hwmon/hwmon2/fan2_input:0`
	got := parseFans(output)
	if len(got) != 1 {
		t.Fatalf("expected 1 active fan (zero dropped), got %d: %+v", len(got), got)
	}
	if got[0].Name != "CPU Fan" || got[0].RPM != 1200 {
		t.Fatalf("fan = %+v, want CPU Fan/1200", got[0])
	}
}

// nct6775 exposes fan*_input with no fan*_label; spinning fans should be named
// by their fanN identifier and stopped (0 RPM) headers dropped.
func TestParseFansWithoutLabelsUsesFanID(t *testing.T) {
	output := `/sys/class/hwmon/hwmon4/fan1_input:0
/sys/class/hwmon/hwmon4/fan2_input:771
/sys/class/hwmon/hwmon4/fan6_input:2209`
	got := parseFans(output)
	if len(got) != 2 {
		t.Fatalf("expected 2 spinning fans, got %d: %+v", len(got), got)
	}
	if got[0].Name != "fan2" || got[0].RPM != 771 {
		t.Fatalf("fan[0] = %+v, want fan2/771", got[0])
	}
	if got[1].Name != "fan6" || got[1].RPM != 2209 {
		t.Fatalf("fan[1] = %+v, want fan6/2209", got[1])
	}
}

// Guards the trap where a new gather command in sysinfo.go is silently rejected
// because it was never added to isAllowedCommand's allowlist.
func TestAllowlistCoversExtendedSensorCommands(t *testing.T) {
	cmds := []string{
		"cat /proc/loadavg",
		"cat /proc/diskstats",
		"free -m | grep -i Swap:",
		"find -L /sys/class/hwmon -maxdepth 2 \\( -name 'fan*_input' -o -name 'fan*_label' \\) -exec grep -H . {} + 2>/dev/null || true",
		"find -L /sys/class/hwmon -maxdepth 2 \\( -name 'temp*_input' -o -name 'temp*_label' \\) -exec grep -H . {} + 2>/dev/null || true",
	}
	for _, c := range cmds {
		if !isAllowedCommand(c) {
			t.Errorf("command issued by sysinfo.go is not allowlisted: %q", c)
		}
	}
}

func TestParseTopCPUUsageKeepsOnlyLastIteration(t *testing.T) {
	// top -bn2 prints the since-boot sample first, then the live one.
	perCore := `%Cpu0  : 90.0 us,  0.0 sy,  0.0 ni, 10.0 id
%Cpu1  : 80.0 us,  0.0 sy,  0.0 ni, 20.0 id
%Cpu0  :  4.0 us,  1.0 sy,  0.0 ni, 95.0 id
%Cpu1  :  0.0 us,  0.0 sy,  0.0 ni, 100.0 id`
	aggregate, cores := parseTopCPUUsage(perCore)
	if len(cores) != 2 || cores[0].UsagePercent != 5.0 || cores[1].UsagePercent != 0 {
		t.Fatalf("cores = %#v, want last iteration only", cores)
	}
	if aggregate != 2.5 {
		t.Fatalf("aggregate = %.1f, want 2.5", aggregate)
	}

	withSummary := `%Cpu(s): 50.0 us,  0.0 sy,  0.0 ni, 50.0 id
%Cpu0  : 50.0 us,  0.0 sy,  0.0 ni, 50.0 id
%Cpu(s):  3.0 us,  0.0 sy,  0.0 ni, 97.0 id
%Cpu0  :  3.0 us,  0.0 sy,  0.0 ni, 97.0 id`
	aggregate, cores = parseTopCPUUsage(withSummary)
	if aggregate != 3.0 || len(cores) != 1 {
		t.Fatalf("aggregate=%.1f cores=%#v, want 3.0 and one core", aggregate, cores)
	}

	busybox := "CPU: 90.0% usr 0.0% sys 0.0% nic 10.0% idle\nCPU:  1.0% usr 0.0% sys 0.0% nic 99.0% idle"
	if aggregate, _ = parseTopCPUUsage(busybox); aggregate != 1.0 {
		t.Fatalf("busybox aggregate = %.1f, want 1.0", aggregate)
	}
}

func TestParseDiskStatsSkipsPartitionsAndStackedDevices(t *testing.T) {
	output := `   8       0 sda 100 0 200 50 80 0 400 30 0 0
   8       1 sda1 100 0 200 50 80 0 400 30 0 0
 259       0 nvme0n1 100 0 800 50 80 0 900 30 0 0
 259       1 nvme0n1p1 100 0 800 50 80 0 900 30 0 0
 259       2 nvme0n1p2 1 0 2 0 3 0 4 0 0 0
 179       0 mmcblk0 1 0 2 0 3 0 4 0 0 0
 179       1 mmcblk0p1 1 0 2 0 3 0 4 0 0 0
 253       0 dm-0 1 0 2 0 3 0 4 0 0 0
   9       0 md0 1 0 2 0 3 0 4 0 0 0`
	var names []string
	for _, d := range parseDiskStats(output) {
		names = append(names, d.Device)
	}
	if got := strings.Join(names, ","); got != "sda,nvme0n1,mmcblk0" {
		t.Fatalf("devices = %s, want sda,nvme0n1,mmcblk0", got)
	}
}

func TestParseDFDedupesDevicesAndSkipsLoop(t *testing.T) {
	output := `/dev/nvme0n1p2  943718400  419430400  524288000  45% /home
/dev/nvme0n1p2  943718400  419430400  524288000  45% /
/dev/loop3         57344      57344          0 100% /snap/core/123
/dev/sdb1     1932735283 1073741824  751619276  60% /mnt/my disk`
	got := parseDF(output)
	if len(got) != 2 {
		t.Fatalf("got %d disks, want 2: %+v", len(got), got)
	}
	if got[0].MountPoint != "/" || got[0].Size != "900G" || got[0].Used != "400G" || got[0].UsedBytes != 419430400*1024 {
		t.Fatalf("disk 0 = %+v, want / 900G/400G with byte counts", got[0])
	}
	if got[1].MountPoint != "/mnt/my disk" {
		t.Fatalf("mount = %q, want mount with space preserved", got[1].MountPoint)
	}
}

func TestHumanKiBMatchesDfStyle(t *testing.T) {
	cases := map[uint64]string{0: "0K", 500: "500K", 1024: "1.0M", 297000: "291M", 2097152: "2.0G", 998244352: "952G", 1572864: "1.5G"}
	for in, want := range cases {
		if got := humanKiB(in); got != want {
			t.Errorf("humanKiB(%d) = %s, want %s", in, got, want)
		}
	}
}

func TestParseCPUFreqAveragesCurrentAndTakesMax(t *testing.T) {
	out := `/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq:2000000
/sys/devices/system/cpu/cpu1/cpufreq/scaling_cur_freq:3000000
/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq:5000000
/sys/devices/system/cpu/cpu1/cpufreq/cpuinfo_max_freq:5100000`
	cur, max := parseCPUFreq(out)
	if cur != 2500 || max != 5100 {
		t.Fatalf("cur=%d max=%d, want 2500/5100", cur, max)
	}
}
func TestParseThermalZonesNumbersRepeatedNames(t *testing.T) {
	output := `/sys/class/thermal/thermal_zone0/type:acpitz
/sys/class/thermal/thermal_zone0/temp:56000
/sys/class/thermal/thermal_zone1/type:acpitz
/sys/class/thermal/thermal_zone1/temp:77000`
	got := parseThermalZones(output)
	if len(got) != 2 || got[0].Name != "acpitz" || got[1].Name != "acpitz 2" {
		t.Fatalf("zones = %+v, want acpitz and acpitz 2", got)
	}
}

func TestParseTopTableReadsColumnsByName(t *testing.T) {
	output := `top - 22:58:34 up 2 days,  3 users,  load average: 5.49, 4.10, 2.97
Tasks: 4893 total,   2 running
%Cpu0  :  4.0 us,  1.0 sy,  0.0 ni, 95.0 id
MiB Mem :  63425.0 total

    PID USER      PR  NI    VIRT    RES    SHR S  %CPU  %MEM     TIME+ COMMAND
2263884 allie     20   0   10.1g   3.2g  120000 S 193.3   5.2  10:11.12 Jarvis
 102146 allie     20   0    1.1g   1.0g   90000 S  32.8   1.2   1:00.00 Web Content
      1 root      20   0   20000   9000    7000 S   0.0   0.0   0:03.00 systemd`
	got := parseTopTable(output, 2)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (limit): %+v", len(got), got)
	}
	if got[0].PID != 2263884 || got[0].CPUPercent != 193.3 || got[0].MemPercent != 5.2 || got[0].Command != "Jarvis" {
		t.Fatalf("row 0 = %+v", got[0])
	}
	if got[1].Command != "Web Content" {
		t.Fatalf("command = %q, want multi-word command kept", got[1].Command)
	}
	if parseTopTable("no table here", 5) != nil {
		t.Fatal("expected nil without a PID header")
	}
}

func TestDropGenericACPIZonesKeepsRealSensors(t *testing.T) {
	got := dropGenericACPIZones([]TemperatureInfo{{Name: "acpitz"}, {Name: "acpitz 2"}, {Name: "iwlwifi"}})
	if len(got) != 1 || got[0].Name != "iwlwifi" {
		t.Fatalf("got %+v, want only iwlwifi", got)
	}
}
