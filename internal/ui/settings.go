package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Threshold is a warn/crit pair for a single metric. A reading at or above Warn
// is a warning; at or above Crit is critical. Both are in the metric's natural
// unit (percent for usage metrics, °C for temperatures).
type Threshold struct {
	Warn float64 `json:"warn"`
	Crit float64 `json:"crit"`
}

// Thresholds holds the alerting limits for every monitored metric. Zero-valued
// pairs (Warn == 0 && Crit == 0) are treated as "unset" and never alert, so a
// partial settings.json still behaves sensibly.
type Thresholds struct {
	CPUPct   Threshold `json:"cpu_pct"`
	RAMPct   Threshold `json:"ram_pct"`
	SwapPct  Threshold `json:"swap_pct"`
	DiskPct  Threshold `json:"disk_pct"`
	TempC    Threshold `json:"temp_c"`
	GPUTempC Threshold `json:"gpu_temp_c"`
	GPUPct   Threshold `json:"gpu_util_pct"`
}

// Settings is the persisted user configuration. It lives next to themes.json and
// layout.json under the rigwatch config directory.
type Settings struct {
	// Interval is the metric refresh interval in seconds. <= 0 means "unset"
	// and the built-in default is used.
	Interval           float64    `json:"interval"`
	DefaultTheme       string     `json:"default_theme"`
	Thresholds         Thresholds `json:"thresholds"`
	ShowExtendedPanels bool       `json:"show_extended_panels"`
}

// DefaultThresholds returns the built-in alerting limits. The temperature pair
// matches the previously hardcoded coloring (warn 70 / crit 85) so behavior is
// unchanged when no settings file exists.
func DefaultThresholds() Thresholds {
	return Thresholds{
		CPUPct:   Threshold{Warn: 85, Crit: 95},
		RAMPct:   Threshold{Warn: 85, Crit: 95},
		SwapPct:  Threshold{Warn: 50, Crit: 80},
		DiskPct:  Threshold{Warn: 85, Crit: 95},
		TempC:    Threshold{Warn: 70, Crit: 85},
		GPUTempC: Threshold{Warn: 80, Crit: 90},
		GPUPct:   Threshold{Warn: 0, Crit: 0}, // high GPU util is normal on a rig; off by default
	}
}

// DefaultSettings returns the configuration used when no settings file is present.
func DefaultSettings() Settings {
	return Settings{
		Interval:           0, // 0 => caller's built-in default
		DefaultTheme:       defaultThemeName,
		Thresholds:         DefaultThresholds(),
		ShowExtendedPanels: true,
	}
}

func settingsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if configDir == "" {
		return "", fmt.Errorf("user config directory unavailable")
	}
	return filepath.Join(configDir, "rigwatch", "settings.json"), nil
}

// LoadSettings reads the persisted settings, falling back to DefaultSettings when
// the file is missing. Any threshold left zero in the file is backfilled from the
// defaults so a hand-edited partial file still alerts on the standard metrics.
func LoadSettings() (Settings, error) {
	path, err := settingsPath()
	if err != nil {
		return DefaultSettings(), err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return DefaultSettings(), err
	}

	// Start from defaults so absent fields keep sensible values.
	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return DefaultSettings(), err
	}
	settings.normalize()
	return settings, nil
}

func SaveSettings(settings Settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	settings.normalize()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// normalize fills in any field left at its zero value with the default, so a
// partial or hand-edited file behaves predictably.
func (s *Settings) normalize() {
	if s.DefaultTheme == "" {
		s.DefaultTheme = defaultThemeName
	} else {
		s.DefaultTheme = ThemeByName(s.DefaultTheme).Name
	}
	def := DefaultThresholds()
	fillThreshold(&s.Thresholds.CPUPct, def.CPUPct)
	fillThreshold(&s.Thresholds.RAMPct, def.RAMPct)
	fillThreshold(&s.Thresholds.SwapPct, def.SwapPct)
	fillThreshold(&s.Thresholds.DiskPct, def.DiskPct)
	fillThreshold(&s.Thresholds.TempC, def.TempC)
	fillThreshold(&s.Thresholds.GPUTempC, def.GPUTempC)
	// GPUPct intentionally not backfilled: its default is off (0/0).
}

func fillThreshold(t *Threshold, def Threshold) {
	if t.Warn == 0 && t.Crit == 0 {
		*t = def
	}
}
