package gpu

import "github.com/allisonhere/rigwatch/internal/gpu/base"

// the list of all available GPU providers
// They will be checked in order, and the first one that detects
// its tooling on the host will be used
// TODO: add support for multiple providers per host
var providers = []base.Provider{
	NvidiaProvider{},
	AMDProvider{},
}

// attempt to detect and query GPUs from all registered providers
// Returns the first successful result, or empty if no GPUs are found
func QueryAll(runCmd base.RunCmdFunc) ([]base.Device, error) {
	for _, p := range providers {
		if p.Detect(runCmd) {
			devices, err := p.Query(runCmd)
			if err != nil {
				continue
			}
			if len(devices) > 0 {
				return devices, nil
			}
		}
	}
	// no GPUs found
	return []base.Device{}, nil
}

func Register(p base.Provider) {
	providers = append(providers, p)
}
