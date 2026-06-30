package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

// LayoutPreferences is the saved quad-view session: which hosts are on the grid
// (in display order) and which page was active. It is persisted next to the
// theme preferences so a saved grid can be restored on the next launch.
type LayoutPreferences struct {
	Hosts []string `json:"hosts"`
	Page  int      `json:"page"`
	// TilesPerPage is the grid page size (2 = dual, 4 = grid). 0 means unset and
	// restores to the default grid size.
	TilesPerPage int `json:"tiles_per_page"`
}

func layoutPreferencesPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if configDir == "" {
		return "", fmt.Errorf("user config directory unavailable")
	}
	return filepath.Join(configDir, "rigwatch", "layout.json"), nil
}

func LoadLayoutPreferences() (LayoutPreferences, error) {
	path, err := layoutPreferencesPath()
	if err != nil {
		return LayoutPreferences{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return LayoutPreferences{}, nil
	}
	if err != nil {
		return LayoutPreferences{}, err
	}
	var prefs LayoutPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return LayoutPreferences{}, err
	}
	return prefs, nil
}

func SaveLayoutPreferences(prefs LayoutPreferences) error {
	path, err := layoutPreferencesPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// RestoreModelFromLayout builds a model that reconnects to a previously saved
// quad layout and lands on the grid once telemetry arrives. It resolves saved
// host names against the hosts currently known from the SSH config, dropping any
// that no longer exist. Returns false when there is no usable saved layout, so
// the caller can fall back to the normal host-picker.
func RestoreModelFromLayout(allHosts []internal.SSHHost, updateInterval time.Duration) (Model, bool) {
	prefs, err := LoadLayoutPreferences()
	if err != nil || len(prefs.Hosts) == 0 {
		return Model{}, false
	}

	hostByName := make(map[string]internal.SSHHost, len(allHosts))
	for _, h := range allHosts {
		hostByName[h.Name] = h
	}

	selected := make([]internal.SSHHost, 0, len(prefs.Hosts))
	for _, name := range prefs.Hosts {
		if h, ok := hostByName[name]; ok {
			selected = append(selected, h)
		}
	}
	if len(selected) == 0 {
		return Model{}, false
	}

	m := InitialModelWithHosts(allHosts, selected, updateInterval)
	m.postConnectScreen = ScreenQuad
	m.quadPage = prefs.Page
	if prefs.TilesPerPage > 0 {
		m.gridTilesPerPage = prefs.TilesPerPage
	}
	return m, true
}
