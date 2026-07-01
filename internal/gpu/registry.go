package gpu

import "github.com/allisonhere/rigwatch/internal/gpu/base"

// the list of all available GPU providers
// Every detected provider is queried so mixed-vendor hosts can report all GPUs.
var providers = []base.Provider{
	NvidiaProvider{},
	AMDProvider{},
}

// QueryAll detects and queries all registered GPU providers.
// Providers that fail are skipped so one broken vendor tool does not hide GPUs
// reported by another provider.
func QueryAll(runCmd base.RunCmdFunc) ([]base.Device, error) {
	var all []base.Device
	for _, p := range providers {
		if p.Detect(runCmd) {
			devices, err := p.Query(runCmd)
			if err != nil {
				continue
			}
			if len(devices) > 0 {
				all = append(all, devices...)
			}
		}
	}
	if all == nil {
		return []base.Device{}, nil
	}
	return all, nil
}

func Register(p base.Provider) {
	providers = append(providers, p)
}
