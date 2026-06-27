package internal

import "testing"

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
