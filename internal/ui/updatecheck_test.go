package ui

import (
	"testing"
	"time"

	"github.com/allisonhere/rigwatch/internal"
)

func TestCheckForUpdatesDisabledDoesNotUseCache(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := saveUpdateCheckCache(updateCheckCache{
		CheckedAt: time.Now(),
		Info:      internal.UpdateInfo{Available: true, CurrentVersion: "v0.0.1", LatestVersion: "v9.9.9"},
	}); err != nil {
		t.Fatalf("saveUpdateCheckCache: %v", err)
	}

	msg := checkForUpdates(Settings{CheckForUpdates: false})()
	info := internal.UpdateInfo(msg.(UpdateCheckMsg))
	if info.Available {
		t.Fatalf("disabled check should not report cached update: %+v", info)
	}
	if info.CurrentVersion != internal.Version {
		t.Fatalf("CurrentVersion = %q, want %q", info.CurrentVersion, internal.Version)
	}
}

func TestCheckForUpdatesUsesFreshCache(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := internal.UpdateInfo{Available: true, CurrentVersion: "v0.0.1", LatestVersion: "v9.9.9"}
	if err := saveUpdateCheckCache(updateCheckCache{CheckedAt: time.Now(), Info: want}); err != nil {
		t.Fatalf("saveUpdateCheckCache: %v", err)
	}

	msg := checkForUpdates(Settings{CheckForUpdates: true, UpdateCheckIntervalHours: 24})()
	got := internal.UpdateInfo(msg.(UpdateCheckMsg))
	if got != want {
		t.Fatalf("cached update info = %+v, want %+v", got, want)
	}
}
