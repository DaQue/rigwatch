package base

// a single GPU/accelerator in vendor-neutral terms
type Device struct {
	Index       int
	Name        string
	VRAMTotal   int // MB
	VRAMUsed    int // MB
	Utilization int // percentage
	PowerDraw   int // watts
	PowerLimit  int // watts
	Temperature int // celsius
	Vendor      string

	// ClockMHz is the current shader/graphics clock and MaxClockMHz the highest
	// clock the card will boost to; both 0 when the host does not expose them.
	ClockMHz    int
	MaxClockMHz int
	// Throttle lists why the card is currently running below its clocks, in
	// plain words ("thermal slowdown", "power cap"). Empty when unthrottled or
	// unknown.
	Throttle []string
}

// Process is one process using a GPU.
type Process struct {
	GPU      string // PCI address or index, to tell cards apart on multi-GPU hosts
	PID      int
	Name     string
	VRAMMB   int
	EngineNs uint64 // cumulative GPU busy time, for deriving utilization; 0 if unknown
	UtilPct  int    // instantaneous utilization; -1 when not reported directly
}

// ProcessLister is implemented by providers that can say which processes are
// using the GPU.
type ProcessLister interface {
	Processes(runCmd RunCmdFunc) []Process
}

type RunCmdFunc func(string) (string, error)

type Provider interface {
	// returns the vendor name (e.g., "nvidia", "amd")
	Name() string

	// returns true if the required tooling exists on the host
	Detect(runCmd RunCmdFunc) bool

	// returns a slice of GPU devices or an error
	Query(runCmd RunCmdFunc) ([]Device, error)
}
