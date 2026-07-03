package gpu

import (
	"fmt"
	"testing"

	"github.com/allisonhere/rigwatch/internal/gpu/base"
)

type fakeProvider struct {
	name    string
	detect  bool
	devices []base.Device
	err     error
}

func (p fakeProvider) Name() string { return p.name }

func (p fakeProvider) Detect(base.RunCmdFunc) bool { return p.detect }

func (p fakeProvider) Query(base.RunCmdFunc) ([]base.Device, error) {
	return p.devices, p.err
}

func withProviders(t *testing.T, testProviders ...base.Provider) {
	t.Helper()
	previous := providers
	providers = testProviders
	t.Cleanup(func() { providers = previous })
}

func TestQueryAllAggregatesDetectedProviders(t *testing.T) {
	withProviders(t,
		fakeProvider{name: "nvidia", detect: true, devices: []base.Device{{Index: 0, Name: "RTX", Vendor: "nvidia"}}},
		fakeProvider{name: "amd", detect: true, devices: []base.Device{{Index: 0, Name: "Radeon", Vendor: "amd"}}},
	)

	devices, err := QueryAll(func(string) (string, error) { return "", nil })
	if err != nil {
		t.Fatalf("QueryAll returned error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices len = %d, want 2: %+v", len(devices), devices)
	}
	if devices[0].Vendor != "nvidia" || devices[1].Vendor != "amd" {
		t.Fatalf("devices = %+v, want both vendors in provider order", devices)
	}
}

func TestQueryAllSkipsFailingProviderAndKeepsOtherDevices(t *testing.T) {
	withProviders(t,
		fakeProvider{name: "broken", detect: true, err: fmt.Errorf("tool failed")},
		fakeProvider{name: "amd", detect: true, devices: []base.Device{{Index: 0, Name: "Radeon", Vendor: "amd"}}},
	)

	devices, err := QueryAll(func(string) (string, error) { return "", nil })
	if err != nil {
		t.Fatalf("QueryAll returned error: %v", err)
	}
	if len(devices) != 1 || devices[0].Vendor != "amd" {
		t.Fatalf("devices = %+v, want only working provider device", devices)
	}
}

func TestQueryAllReturnsEmptyWhenNoProviderReportsDevices(t *testing.T) {
	withProviders(t,
		fakeProvider{name: "undetected", detect: false, devices: []base.Device{{Name: "hidden"}}},
		fakeProvider{name: "empty", detect: true},
	)

	devices, err := QueryAll(func(string) (string, error) { return "", nil })
	if err != nil {
		t.Fatalf("QueryAll returned error: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("devices = %+v, want empty", devices)
	}
}
