package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/allisonhere/rigwatch/internal"
	tea "github.com/charmbracelet/bubbletea"
)

type updateCheckCache struct {
	CheckedAt time.Time           `json:"checked_at"`
	Info      internal.UpdateInfo `json:"info"`
}

func updateCheckCachePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if configDir == "" {
		return "", fmt.Errorf("user config directory unavailable")
	}
	return filepath.Join(configDir, "rigwatch", "update_check.json"), nil
}

func loadUpdateCheckCache() (updateCheckCache, error) {
	path, err := updateCheckCachePath()
	if err != nil {
		return updateCheckCache{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return updateCheckCache{}, nil
	}
	if err != nil {
		return updateCheckCache{}, err
	}
	var cache updateCheckCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return updateCheckCache{}, err
	}
	return cache, nil
}

func saveUpdateCheckCache(cache updateCheckCache) error {
	path, err := updateCheckCachePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func checkForUpdates(settings Settings) tea.Cmd {
	return func() tea.Msg {
		if !settings.CheckForUpdates {
			return UpdateCheckMsg(internal.UpdateInfo{Available: false, CurrentVersion: internal.Version})
		}

		interval := time.Duration(settings.UpdateCheckIntervalHours) * time.Hour
		if interval <= 0 {
			interval = 24 * time.Hour
		}
		if cache, err := loadUpdateCheckCache(); err == nil && !cache.CheckedAt.IsZero() && time.Since(cache.CheckedAt) < interval {
			return UpdateCheckMsg(cache.Info)
		}

		info := internal.CheckForUpdates()
		if info.LatestVersion != "" || internal.Version == "dev" {
			_ = saveUpdateCheckCache(updateCheckCache{CheckedAt: time.Now(), Info: info})
		}
		return UpdateCheckMsg(info)
	}
}
